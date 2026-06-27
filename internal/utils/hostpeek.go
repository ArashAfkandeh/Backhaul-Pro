package utils

import (
    "bytes"
    "encoding/binary"
    "errors"
    "io"
    "net"
    "strings"
    "time"
)

// peekConn wraps an underlying net.Conn and serves the initially-read
// buffered bytes first, then falls through to the underlying connection.
type peekConn struct {
    net.Conn
    buf *bytes.Reader
}

func (p *peekConn) Read(b []byte) (int, error) {
    if p.buf != nil && p.buf.Len() > 0 {
        n, _ := p.buf.Read(b)
        if n > 0 {
            return n, nil
        }
        // buffer exhausted
        p.buf = nil
    }
    return p.Conn.Read(b)
}

// PeekResult contains the result of peeking into a connection for host
// information. If Conn is non-nil it should be used in place of the original
// conn (it returns the peeked bytes first).
type PeekResult struct {
    Host string
    IsTLS bool
    Conn net.Conn
}

var ErrNoData = errors.New("no data available for peek")

// PeekHostFromConn attempts to read up to maxPeek bytes from conn with the
// specified timeout and extract either an HTTP Host header or a TLS SNI from
// the ClientHello. It returns a PeekResult. If no hostname is found the Host
// field will be empty. The returned Conn must be used for further reads so
// that the initial bytes are not lost; if no bytes were read the original
// conn is returned unchanged.
func PeekHostFromConn(conn net.Conn, timeout time.Duration, maxPeek int) (PeekResult, error) {
    if maxPeek <= 0 {
        maxPeek = 8192
    }
    if timeout <= 0 {
        timeout = 200 * time.Millisecond
    }

    // overall deadline
    deadline := time.Now().Add(timeout)
    defer conn.SetReadDeadline(time.Time{})

    // buffer to accumulate reads
    var dataBuf []byte
    tmp := make([]byte, 2048)

    for {
        // try parse with whatever we have so far
        if host, ok := parseTLSSNI(dataBuf); ok {
            p := &peekConn{Conn: conn, buf: bytes.NewReader(dataBuf)}
            return PeekResult{Host: strings.ToLower(host), IsTLS: true, Conn: p}, nil
        }
        if host, ok := parseHTTPHost(dataBuf); ok {
            p := &peekConn{Conn: conn, buf: bytes.NewReader(dataBuf)}
            return PeekResult{Host: strings.ToLower(host), IsTLS: false, Conn: p}, nil
        }

        // stop if we've read enough
        if len(dataBuf) >= maxPeek {
            p := &peekConn{Conn: conn, buf: bytes.NewReader(dataBuf)}
            return PeekResult{Host: "", IsTLS: false, Conn: p}, nil
        }

        // remaining time
        rem := time.Until(deadline)
        if rem <= 0 {
            if len(dataBuf) == 0 {
                return PeekResult{Host: "", IsTLS: false, Conn: conn}, ErrNoData
            }
            p := &peekConn{Conn: conn, buf: bytes.NewReader(dataBuf)}
            return PeekResult{Host: "", IsTLS: false, Conn: p}, nil
        }

        // set a short read deadline so we don't block beyond remaining time
        rd := rem
        if rd > 50*time.Millisecond {
            rd = 50 * time.Millisecond
        }
        _ = conn.SetReadDeadline(time.Now().Add(rd))

        n, err := conn.Read(tmp)
        if n > 0 {
            if len(dataBuf)+n > maxPeek {
                n = maxPeek - len(dataBuf)
            }
            dataBuf = append(dataBuf, tmp[:n]...)
            continue
        }

        if err != nil {
            if ne, ok := err.(net.Error); ok && ne.Timeout() {
                // try again until overall deadline
                continue
            }
            if err == io.EOF {
                if len(dataBuf) == 0 {
                    return PeekResult{Host: "", IsTLS: false, Conn: conn}, ErrNoData
                }
                p := &peekConn{Conn: conn, buf: bytes.NewReader(dataBuf)}
                return PeekResult{Host: "", IsTLS: false, Conn: p}, nil
            }
            return PeekResult{}, err
        }
    }
}

// parseHTTPHost looks for an HTTP Host header in the provided data and
// returns it if present. It expects CRLF line endings.
func parseHTTPHost(data []byte) (string, bool) {
    // quick check for HTTP methods (GET/POST/PUT/HEAD/OPTIONS/DELETE/CONNECT/PATCH)
    methods := [][]byte{[]byte("GET "), []byte("POST "), []byte("PUT "), []byte("HEAD "), []byte("OPTIONS "), []byte("DELETE "), []byte("CONNECT "), []byte("PATCH ")}
    isHTTP := false
    for _, m := range methods {
        if len(data) >= len(m) && bytes.HasPrefix(data, m) {
            isHTTP = true
            break
        }
    }
    if !isHTTP {
        return "", false
    }

    // search for end of headers
    end := bytes.Index(data, []byte("\r\n\r\n"))
    if end == -1 {
        // not enough data to include headers
        // still try to find a Host header in what we have
        end = len(data)
    }

    headers := data[:end]
    // naive header parsing: look for lines starting with "Host:"
    lines := bytes.Split(headers, []byte("\r\n"))
    for _, line := range lines {
        line = bytes.TrimSpace(line)
        if len(line) >= 5 && (bytes.HasPrefix(bytes.ToLower(line), []byte("host:"))) {
            // split on ':' to get value
            idx := bytes.IndexByte(line, ':')
            if idx == -1 {
                continue
            }
            host := strings.TrimSpace(string(line[idx+1:]))
            // strip optional port
            if h, _, err := net.SplitHostPort(host); err == nil {
                return h, true
            }
            return host, true
        }
    }
    return "", false
}

// parseTLSSNI attempts to parse a TLS ClientHello from the given buffer and
// extract the SNI (server_name) if present. It is tolerant to buffers that
// may not contain the full ClientHello and returns ok=false in that case.
func parseTLSSNI(data []byte) (string, bool) {
    // Need at least TLS record header
    if len(data) < 5 {
        return "", false
    }
    // TLS record type 22 = handshake
    if data[0] != 0x16 {
        return "", false
    }
    // record length
    if len(data) < 5 {
        return "", false
    }
    recLen := int(binary.BigEndian.Uint16(data[3:5]))
    if len(data) < 5+recLen {
        // truncated
        // but we still might have enough for ClientHello parsing – continue
    }

    // handshake header starts at byte 5
    if len(data) < 6 {
        return "", false
    }
    // handshake type 1 = ClientHello
    if data[5] != 0x01 {
        return "", false
    }
    // handshake length is 3 bytes
    if len(data) < 9 {
        return "", false
    }
    // position after handshake header
    pos := 5 + 4 // handshake type (1) + length (3)

    // skip: client version (2) + random (32)
    if len(data) < pos+2+32 {
        return "", false
    }
    pos += 2 + 32

    // session id
    if len(data) < pos+1 {
        return "", false
    }
    sidLen := int(data[pos])
    pos += 1
    if len(data) < pos+sidLen {
        return "", false
    }
    pos += sidLen

    // cipher suites
    if len(data) < pos+2 {
        return "", false
    }
    csLen := int(binary.BigEndian.Uint16(data[pos : pos+2]))
    pos += 2
    if len(data) < pos+csLen {
        return "", false
    }
    pos += csLen

    // compression methods
    if len(data) < pos+1 {
        return "", false
    }
    compLen := int(data[pos])
    pos += 1
    if len(data) < pos+compLen {
        return "", false
    }
    pos += compLen

    // extensions length
    if len(data) < pos+2 {
        return "", false
    }
    extLen := int(binary.BigEndian.Uint16(data[pos : pos+2]))
    pos += 2
    if len(data) < pos+extLen {
        // truncated but proceed with available
    }

    endExt := pos + extLen
    for pos+4 <= len(data) && pos+4 <= endExt {
        extType := int(binary.BigEndian.Uint16(data[pos : pos+2]))
        extSize := int(binary.BigEndian.Uint16(data[pos+2 : pos+4]))
        pos += 4
        if pos+extSize > len(data) {
            return "", false
        }
        if extType == 0 { // server_name
            // server_name extension structure: list length (2), then list of name entries
            if extSize < 2 {
                return "", false
            }
            listLen := int(binary.BigEndian.Uint16(data[pos : pos+2]))
            pos += 2
            endList := pos + listLen
            for pos+3 <= endList && pos+3 <= len(data) {
                nameType := data[pos]
                nameLen := int(binary.BigEndian.Uint16(data[pos+1 : pos+3]))
                pos += 3
                if pos+nameLen > len(data) || pos+nameLen > endList {
                    return "", false
                }
                if nameType == 0 {
                    name := string(data[pos : pos+nameLen])
                    // strip port if present
                    if h, _, err := net.SplitHostPort(name); err == nil {
                        return h, true
                    }
                    return name, true
                }
                pos += nameLen
            }
            return "", false
        }
        pos += extSize
    }

    return "", false
}
