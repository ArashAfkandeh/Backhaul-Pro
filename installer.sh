#!/bin/bash

# Colors for RGBY scheme
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
WHITE='\033[1;37m'
NC='\033[0m' # No Color

# Function to clear screen and show header
clear_screen() {
    clear
    echo -e "${BLUE}=== Backhaul Pro Configuration Script ===${NC}"
    echo -e "${CYAN}Step $1: $2${NC}"
    echo -e "${PURPLE}=========================================${NC}"
    echo ""
}

# Function to show interactive prompt
show_prompt() {
    echo -e "${YELLOW}╔════════════════════════════════════════╗${NC}"
    echo -e "${YELLOW}║           INTERACTIVE PROMPT           ║${NC}"
    echo -e "${YELLOW}╚════════════════════════════════════════╝${NC}"
}

# Function to show success message
show_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

# Function to show warning message
show_warning() {
    echo -e "${YELLOW}⚠ $1${NC}"
}

# Function to show error message
show_error() {
    echo -e "${RED}✗ $1${NC}"
}

# Function to show info message
show_info() {
    echo -e "${CYAN}ℹ $1${NC}"
}

# Function to find next available config file number
find_next_config_number() {
    local base_name="$1"
    local counter=1
    local config_file
    
    while true; do
        if [ $counter -eq 1 ]; then
            config_file="$BACKHAUL_DIR/${base_name}.toml"
        else
            config_file="$BACKHAUL_DIR/${base_name}${counter}.toml"
        fi
        
        if [ ! -f "$config_file" ]; then
            echo $counter
            return
        fi
        ((counter++))
    done
}

# Function to check if a port is available
check_port_available() {
    local port="$1"
    if ss -tuln | grep -q ":$port "; then
        return 1  # Port is in use
    else
        return 0  # Port is available
    fi
}

# Function to extract local ports from entry for checking availability
extract_local_ports() {
    local entry="$1"
    local local_part

    # Get the local part before = or :
    if [[ $entry == *"="* ]]; then
        local_part="${entry%%=*}"
    elif [[ $entry == *":"* ]]; then
        local_part="${entry%%:*}"
    else
        local_part="$entry"
    fi

    # If local_part has IP:port, extract port
    if [[ $local_part == *":"* ]]; then
        local_port="${local_part##*:}"
    else
        local_port="$local_part"
    fi

    # If it's a range, expand to list of ports
    if [[ $local_port == *"-"* ]]; then
        local start="${local_port%%-*}"
        local end="${local_port##*-}"
        for ((p=start; p<=end; p++)); do
            echo "$p"
        done
    else
        echo "$local_port"
    fi
}

# Function to validate IP address
validate_ip() {
    local ip="$1"
    if [[ $ip =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]]; then
        IFS='.' read -r -a octets <<< "$ip"
        for octet in "${octets[@]}"; do
            if (( octet < 0 || octet > 255 )); then
                return 1
            fi
        done
        return 0
    else
        return 1
    fi
}

# Function to validate port entry more strictly
validate_port_entry() {
    local entry="$1"

    # Regex for local part: (ip:)? (port|range)
    local local_regex='^(([0-9]{1,3}\.){3}[0-9]{1,3}:)?([0-9]+(-[0-9]+)?)$'

    # Regex for forward part: (port | ip:port)
    local forward_regex='^(([0-9]{1,3}\.){3}[0-9]{1,3}:)?[0-9]+$'

    # Split entry
    if [[ $entry == *"="* ]]; then
        local_part="${entry%%=*}"
        forward_part="${entry#*=}"
        separator="="
    elif [[ $entry == *":"* ]]; then
        local_part="${entry%%:*}"
        forward_part="${entry#*:}"
        separator=":"
    else
        local_part="$entry"
        forward_part=""
        separator=""
    fi

    # Validate local part
    if ! [[ $local_part =~ $local_regex ]]; then
        return 1
    fi

    # If local has IP, validate it
    if [[ $local_part == *":"* ]]; then
        local_ip="${local_part%%:*}"
        if ! validate_ip "$local_ip"; then
            return 1
        fi
        local_port_part="${local_part#*:}"
    else
        local_port_part="$local_part"
    fi

    # Validate local ports/ranges
    if [[ $local_port_part == *"-"* ]]; then
        start="${local_port_part%%-*}"
        end="${local_port_part##*-}"
        if (( start >= end || start < 1 || end > 65535 )); then
            return 1
        fi
    else
        if (( local_port_part < 1 || local_port_part > 65535 )); then
            return 1
        fi
    fi

    # If there's forward part, validate it
    if [ -n "$forward_part" ]; then
        if ! [[ $forward_part =~ $forward_regex ]]; then
            return 1
        fi
        if [[ $forward_part == *":"* ]]; then
            fwd_ip="${forward_part%%:*}"
            if ! validate_ip "$fwd_ip"; then
                return 1
            fi
            fwd_port="${forward_part#*:}"
        else
            fwd_port="$forward_part"
        fi
        if (( fwd_port < 1 || fwd_port > 65535 )); then
            return 1
        fi
    fi

    return 0
}

# Function to get current server IP (for Show Connection Info)
get_server_ip() {
    curl -s ifconfig.me || echo "Unable to fetch IP automatically"
}

# Function to read TOML value
read_toml_value() {
    local file="$1"
    local key="$2"
    grep -oP "^$key\s*=\s*\K.*" "$file" | sed -e 's/^"//' -e 's/"$//' -e 's/^\[//' -e 's/\]$//'
}

# Function to edit config interactively
edit_config() {
    local config_file="$1"
    local service_name="$2"

    # Determine config type
    if grep -q "\[server\]" "$config_file"; then
        CONFIG_TYPE="server"
    else
        CONFIG_TYPE="client"
    fi

    clear_screen "Edit" "Editing Configuration: $config_file"

    # Read current values
    CURRENT_BIND_ADDR=$(read_toml_value "$config_file" "bind_addr")
    CURRENT_REMOTE_ADDR=$(read_toml_value "$config_file" "remote_addr")
    CURRENT_TRANSPORT=$(read_toml_value "$config_file" "transport")
    CURRENT_TOKEN=$(read_toml_value "$config_file" "token")
    CURRENT_WEB_PORT=$(read_toml_value "$config_file" "web_port")
    CURRENT_PORTS=$(read_toml_value "$config_file" "ports")

    # Extract ports from array string
    if [ -n "$CURRENT_PORTS" ]; then
        # Parse the ports array into list of strings
        CURRENT_PORTS_ARRAY=($(echo "$CURRENT_PORTS" | tr ',' '\n' | sed 's/ //g' | tr -d '"'))
    else
        CURRENT_PORTS_ARRAY=()
    fi

    # Extract ports from bind_addr or remote_addr
    if [ "$CONFIG_TYPE" = "server" ]; then
        CURRENT_TUNNEL_PORT=$(echo "$CURRENT_BIND_ADDR" | cut -d':' -f2)
    else
        CURRENT_SERVER_IP=$(echo "$CURRENT_REMOTE_ADDR" | cut -d':' -f1)
        CURRENT_TUNNEL_PORT=$(echo "$CURRENT_REMOTE_ADDR" | cut -d':' -f2)
    fi

    show_prompt
    echo -e "${WHITE}Editing ${CONFIG_TYPE^} Configuration${NC}"
    echo -e "${YELLOW}Current values shown as defaults.${NC}"
    echo ""

    if [ "$CONFIG_TYPE" = "client" ]; then
        # Server IP
        while true; do
            read -p "$(echo -e "${YELLOW}Enter server IP address (current: $CURRENT_SERVER_IP): ${NC}")" SERVER_IP
            SERVER_IP=${SERVER_IP:-$CURRENT_SERVER_IP}
            if validate_ip "$SERVER_IP"; then
                break
            else
                show_error "Invalid IP format. Please enter a valid IPv4 address."
            fi
        done
        show_success "Server IP set to: $SERVER_IP"
        echo ""
    fi

    # Tunnel port
    while true; do
        read -p "$(echo -e "${YELLOW}Enter tunnel port (current: $CURRENT_TUNNEL_PORT): ${NC}")" TUNNEL_PORT
        TUNNEL_PORT=${TUNNEL_PORT:-$CURRENT_TUNNEL_PORT}
        if [[ ! "$TUNNEL_PORT" =~ ^[0-9]+$ ]] || [ "$TUNNEL_PORT" -lt 1 ] || [ "$TUNNEL_PORT" -gt 65535 ]; then
            show_error "Invalid port number. Please enter a number between 1-65535."
            continue
        fi
        if ! check_port_available "$TUNNEL_PORT"; then
            show_error "Port $TUNNEL_PORT is already in use. Please choose another."
            continue
        fi
        break
    done
    show_success "Tunnel port set to: $TUNNEL_PORT"
    echo ""

    # Protocol
    echo -e "${WHITE}Protocol Selection (current: $CURRENT_TRANSPORT)${NC}"
    echo -e "${GREEN}1) udp${NC}"
    echo -e "${GREEN}2) tcp${NC}"
    echo -e "${GREEN}3) tcpmux${NC}"
    echo -e "${GREEN}4) ws${NC}"
    echo -e "${GREEN}5) wss${NC}"
    echo -e "${GREEN}6) wsmux${NC}"
    echo -e "${GREEN}7) wssmux${NC}"
    while true; do
        read -p "$(echo -e "${YELLOW}Enter protocol choice (1-7, default current): ${NC}")" protocol_choice
        if [ -z "$protocol_choice" ]; then
            PROTOCOL="$CURRENT_TRANSPORT"
            break
        fi
        case $protocol_choice in
            1) PROTOCOL="udp"; break ;;
            2) PROTOCOL="tcp"; break ;;
            3) PROTOCOL="tcpmux"; break ;;
            4) PROTOCOL="ws"; break ;;
            5) PROTOCOL="wss"; break ;;
            6) PROTOCOL="wsmux"; break ;;
            7) PROTOCOL="wssmux"; break ;;
            *) show_error "Invalid choice." ;;
        esac
    done
    show_success "Protocol set to: $PROTOCOL"
    echo ""

    # Token
    while true; do
        read -p "$(echo -e "${YELLOW}Enter token (current: $CURRENT_TOKEN): ${NC}")" RANDOM_TOKEN
        RANDOM_TOKEN=${RANDOM_TOKEN:-$CURRENT_TOKEN}
        if [ -z "$RANDOM_TOKEN" ]; then
            show_error "Token cannot be empty."
            continue
        fi
        break
    done
    show_success "Token set."
    echo ""

    # Web port
    while true; do
        read -p "$(echo -e "${YELLOW}Enter web port (current: $CURRENT_WEB_PORT): ${NC}")" WEB_PORT
        WEB_PORT=${WEB_PORT:-$CURRENT_WEB_PORT}
        if [[ ! "$WEB_PORT" =~ ^[0-9]+$ ]] || [ "$WEB_PORT" -lt 1 ] || [ "$WEB_PORT" -gt 65535 ]; then
            show_error "Invalid port number."
            continue
        fi
        if ! check_port_available "$WEB_PORT"; then
            show_error "Port $WEB_PORT is already in use."
            continue
        fi
        break
    done
    show_success "Web port set to: $WEB_PORT"
    echo ""

    if [ "$CONFIG_TYPE" = "server" ]; then
        # Ports
        echo -e "${WHITE}Ports Configuration (current: ${CURRENT_PORTS_ARRAY[*]})${NC}"
        echo -e "${YELLOW}Enter new port entries (leave empty to keep current, or enter new list). Press Enter without value to finish adding.${NC}"
        echo -e "${YELLOW}Format examples: 443-600, 443-600:5201, 443-600=1.1.1.1:5201, 443, 4000=5000, 127.0.0.2:443=5201, etc.${NC}"
        ports=()
        port_count=1
        while true; do
            read -p "$(echo -e "${YELLOW}Port entry $port_count: ${NC}")" port_input
            if [ -z "$port_input" ]; then
                if [ ${#ports[@]} -eq 0 ] && [ ${#CURRENT_PORTS_ARRAY[@]} -gt 0 ]; then
                    ports=("${CURRENT_PORTS_ARRAY[@]}")
                fi
                if [ ${#ports[@]} -eq 0 ]; then
                    show_error "At least one port entry is required."
                    continue
                fi
                break
            fi

            # Validate entry
            if ! validate_port_entry "$port_input"; then
                show_error "Invalid port entry format or values. Please check examples and ensure ports are 1-65535 and IPs are valid."
                continue
            fi

            # Check local ports availability
            local_ports=$(extract_local_ports "$port_input")
            port_in_use=false
            for p in $local_ports; do
                if ! check_port_available "$p"; then
                    show_error "Local port $p in entry '$port_input' is already in use."
                    port_in_use=true
                    break
                fi
            done
            if $port_in_use; then
                continue
            fi

            ports+=("$port_input")
            show_success "Port entry added: $port_input"
            ((port_count++))
        done
        echo ""
    fi

    # Write new config
    if [ "$CONFIG_TYPE" = "server" ]; then
        cat > "$config_file" << EOF
[server]
bind_addr = "0.0.0.0:$TUNNEL_PORT"
transport = "$PROTOCOL"
token = "$RANDOM_TOKEN"
web_port = $WEB_PORT
ports = [
EOF
        for port_entry in "${ports[@]}"; do
            echo "\"$port_entry\"," >> "$config_file"
        done
        sed -i '$ s/,$//' "$config_file"
        echo "]" >> "$config_file"
    else
        cat > "$config_file" << EOF
[client]
remote_addr = "$SERVER_IP:$TUNNEL_PORT"
transport = "$PROTOCOL"
token = "$RANDOM_TOKEN"
web_port = $WEB_PORT
EOF
    fi

    show_success "Configuration updated."

    # Restart service
    sudo systemctl restart "$service_name"
    show_success "Service $service_name restarted."
}

# Function to uninstall a specific service
uninstall_service() {
    local selected_service="$1"

    service_file_path="/etc/systemd/system/$selected_service"

    clear_screen "Uninstalling" "$selected_service"

    echo -e "${WHITE}Stopping service: ${CYAN}$selected_service${NC}"
    sudo systemctl stop "$selected_service" &> /dev/null
    show_success "Service stopped."

    echo -e "${WHITE}Disabling service: ${CYAN}$selected_service${NC}"
    sudo systemctl disable "$selected_service" &> /dev/null
    show_success "Service disabled."

    # Find the config file from the service definition
    config_file=""
    if [ -f "$service_file_path" ]; then
        config_file=$(grep -oP 'ExecStart=.* -c \K\S+' "$service_file_path")
    fi

    echo -e "${WHITE}Removing service file: ${CYAN}$service_file_path${NC}"
    sudo rm -f "$service_file_path"
    show_success "Service file removed."

    if [ -n "$config_file" ] && [ -f "$config_file" ]; then
        echo -e "${WHITE}Removing configuration file: ${CYAN}$config_file${NC}"
        sudo rm -f "$config_file"
        show_success "Configuration file removed."
    else
        show_warning "Associated configuration file not found or already removed."
    fi

    echo -e "${WHITE}Reloading systemd daemon...${NC}"
    sudo systemctl daemon-reload
    show_success "Systemd daemon reloaded."
    echo ""

    show_info "Uninstall for ${CYAN}$selected_service${NC} completed."
    echo ""
}

# Function to show connection info (for server configs)
show_connection_info() {
    local config_file="$1"

    if ! grep -q "\[server\]" "$config_file"; then
        show_error "This is a client config. Connection info is for servers only."
        return
    fi

    local server_ip=$(get_server_ip)
    local tunnel_port=$(read_toml_value "$config_file" "bind_addr" | cut -d':' -f2)
    local protocol=$(read_toml_value "$config_file" "transport")
    local token=$(read_toml_value "$config_file" "token")
    local web_port=$(read_toml_value "$config_file" "web_port")
    local ports=$(read_toml_value "$config_file" "ports" | tr ',' '\n' | sed 's/ //g' | tr -d '"')

    clear_screen "Info" "Connection Information"
    echo -e "${GREEN}╔══════════════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║                   CLIENT CONNECTION INFO                 ║${NC}"
    echo -e "${GREEN}╚══════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo -e "${WHITE}Server IP: ${CYAN}$server_ip${NC}"
    echo -e "${WHITE}Tunnel Port: ${CYAN}$tunnel_port${NC}"
    echo -e "${WHITE}Protocol: ${CYAN}$protocol${NC}"
    echo -e "${WHITE}Token: ${CYAN}$token${NC}"
    echo -e "${WHITE}Web Port: ${CYAN}$web_port${NC}"
    echo -e "${WHITE}Ports:${NC}"
    for port in $ports; do
        echo -e "${CYAN}  - $port${NC}"
    done
    echo ""
    echo -e "${YELLOW}Use the above information to configure the client.${NC}"
    echo -e "${YELLOW}Example client config:${NC}"
    echo "[client]"
    echo "remote_addr = \"$server_ip:$tunnel_port\""
    echo "transport = \"$protocol\""
    echo "token = \"$token\""
    echo "web_port = $web_port"
}

# Function for management menu
management_menu() {
    # Find relevant services
    services=( $(systemctl list-units --type=service --all | grep -o -E 'backhaul(_pro)?[0-9]*\.service|utunnel[0-9]*\.service' | sort -u) )

    if [ ${#services[@]} -eq 0 ]; then
        show_info "No Backhaul services found."
        exit 0
    fi

    while true; do
        clear_screen "Manage" "Central Management Menu"

        show_prompt
        echo -e "${WHITE}Available Services:${NC}"
        for i in "${!services[@]}"; do
            printf "${GREEN}%d) %s\n${NC}" $((i+1)) "${services[$i]}"
        done
        echo -e "${GREEN}0) Exit${NC}"
        echo ""

        read -p "$(echo -e "${YELLOW}Select service (1-${#services[@]}) or 0 to exit: ${NC}")" choice
        if [ "$choice" = "0" ]; then
            exit 0
        fi
        if [[ ! "$choice" =~ ^[0-9]+$ ]] || [ "$choice" -lt 1 ] || [ "$choice" -gt ${#services[@]} ]; then
            show_error "Invalid choice."
            sleep 2
            continue
        fi

        selected_service=${services[$((choice-1))]}
        config_file=$(grep -oP 'ExecStart=.* -c \K\S+' "/etc/systemd/system/$selected_service" 2>/dev/null)

        if [ -z "$config_file" ]; then
            show_error "Could not find config file for $selected_service."
            sleep 2
            continue
        fi

        while true; do
            clear_screen "Manage" "Managing $selected_service"

            show_prompt
            echo -e "${GREEN}1) Status${NC}"
            echo -e "${GREEN}2) Logs (live)${NC}"
            echo -e "${GREEN}3) Restart${NC}"
            echo -e "${GREEN}4) Edit Config${NC}"
            echo -e "${GREEN}5) Show Connection Info (if server)${NC}"
            echo -e "${GREEN}6) Uninstall this service${NC}"
            echo -e "${GREEN}0) Back to services list${NC}"
            echo ""

            read -p "$(echo -e "${YELLOW}Enter option (0-6): ${NC}")" option
            case $option in
                1)
                    sudo systemctl status "$selected_service"
                    read -p "$(echo -e "${YELLOW}Press Enter to continue...${NC}")"
                    ;;
                2)
                    sudo journalctl -u "$selected_service" -f
                    ;;
                3)
                    sudo systemctl restart "$selected_service"
                    show_success "Service restarted."
                    sleep 2
                    ;;
                4)
                    edit_config "$config_file" "$selected_service"
                    read -p "$(echo -e "${YELLOW}Press Enter to continue...${NC}")"
                    ;;
                5)
                    show_connection_info "$config_file"
                    read -p "$(echo -e "${YELLOW}Press Enter to continue...${NC}")"
                    ;;
                6)
                    read -p "$(echo -e "${YELLOW}Are you sure you want to uninstall $selected_service? (y/N): ${NC}")" confirm
                    if [[ "$confirm" =~ ^[Yy]$ ]]; then
                        uninstall_service "$selected_service"
                        # Refresh services list
                        services=( $(systemctl list-units --type=service --all | grep -o -E 'backhaul(_pro)?[0-9]*\.service|utunnel[0-9]*\.service' | sort -u) )
                        read -p "$(echo -e "${YELLOW}Press Enter to continue...${NC}")"
                    fi
                    ;;
                0)
                    break
                    ;;
                *)
                    show_error "Invalid option."
                    sleep 2
                    ;;
            esac
        done
    done
}

BACKHAUL_DIR="/root/backhaul_pro"
DOWNLOAD_URL="https://github.com/ArashAfkandeh/Backhaul/releases/download/Backhaul_Pro/backhaul_pro.tar.gz"
SERVICE_NAME="backhaul_pro"
CONFIG_FILE="$BACKHAUL_DIR/config.toml"
ARCHIVE="/root/backhaul_pro.tar.gz"

# Install bh-p command if not exists
install_bh_p() {
    local script_path="/usr/local/bin/bh-p"
    if [ ! -f "$script_path" ]; then
        cp "$0" "$script_path"
        chmod +x "$script_path"
        show_success "bh-p command installed."
    fi
}

# Main logic
script_name=$(basename "$0")
if [ "$script_name" = "bh-p" ] && [ -z "$1" ]; then
    management_menu
    exit 0
fi

case "$1" in
    uninstall)
        clear_screen "Uninstall" "Removing Backhaul Pro Service"

        # Find relevant services
        services=( $(systemctl list-units --type=service --all | grep -o -E 'backhaul(_pro)?[0-9]*\.service|utunnel[0-9]*\.service' | sort -u) )

        if [ ${#services[@]} -eq 0 ]; then
            show_info "No Backhaul or related services found to uninstall."
            exit 0
        fi

        show_prompt
        echo -e "${WHITE}Please select the service to uninstall:${NC}"
        for i in "${!services[@]}"; do
            printf "${GREEN}%d) %s\n${NC}" $((i+1)) "${services[$i]}"
        done
        echo -e "${GREEN}a) Uninstall all services${NC}"
        echo ""

        while true; do
            read -p "$(echo -e "${YELLOW}Enter your choice (1-${#services[@]}) or 'a' for all: ${NC}")" choice
            if [[ "$choice" = "a" ]]; then
                selected_services=("${services[@]}")
                break
            elif [[ "$choice" =~ ^[0-9]+$ ]] && [ "$choice" -ge 1 ] && [ "$choice" -le ${#services[@]} ]; then
                selected_services=("${services[$((choice-1))]}")
                break
            else
                show_error "Invalid choice."
            fi
        done

        for selected_service in "${selected_services[@]}"; do
            uninstall_service "$selected_service"
        done

        # Check for complete cleanup
        if [ -d "$BACKHAUL_DIR" ]; then
            if ! ls "$BACKHAUL_DIR"/config*.toml &>/dev/null && [ ! -f "$BACKHAUL_DIR/backhaul_pro" ]; then
                read -p "$(echo -e "${YELLOW}No configs or binary remain. Remove '$BACKHAUL_DIR'? (y/N): ${NC}")" remove_dir_choice
                if [[ "$remove_dir_choice" =~ ^[Yy]$ ]]; then
                    sudo rm -rf "$BACKHAUL_DIR"
                    show_success "Directory '$BACKHAUL_DIR' removed."
                fi
            elif ! ls "$BACKHAUL_DIR"/config*.toml &>/dev/null; then
                read -p "$(echo -e "${YELLOW}No configs remain. Remove binary and directory? (y/N): ${NC}")" remove_dir_choice
                if [[ "$remove_dir_choice" =~ ^[Yy]$ ]]; then
                    sudo rm -rf "$BACKHAUL_DIR"
                    show_success "Directory '$BACKHAUL_DIR' and binary removed."
                fi
            fi
        fi

        # Remove bh-p if no services left
        if [ ${#services[@]} -eq 0 ]; then
            rm -f /usr/local/bin/bh-p
            show_success "bh-p command removed."
        fi

        exit 0
        ;;
    manage)
        management_menu
        exit 0
        ;;
    *)
        # Check for local archive and ask for installation mode
        install_mode="online"
        if [ -f "$ARCHIVE" ]; then
            clear_screen "Package Mode" "Package Installation Mode Selection"
            show_prompt
            echo -e "${WHITE}Local archive '$ARCHIVE' found.${NC}"
            echo -e "${WHITE}Choose how to install required packages:${NC}"
            echo -e "${GREEN}1) Online (default)${NC}"
            echo -e "${GREEN}2) Offline (local from archive)${NC}"
            echo ""

            while true; do
                read -p "$(echo -e "${YELLOW}Enter your choice (1/2): ${NC}")" mode_choice
                mode_choice=${mode_choice:-1}
                
                case $mode_choice in
                    1)
                        install_mode="online"
                        show_info "Online installation selected."
                        break
                        ;;
                    2)
                        install_mode="offline"
                        show_info "Offline (local) installation selected."
                        break
                        ;;
                    *)
                        show_error "Invalid choice. Please enter '1' or '2'."
                        ;;
                esac
            done
        fi

        # Install required packages
        if [ "$install_mode" = "online" ]; then
            clear_screen "0" "System Preparation"
            echo -e "${WHITE}Updating package list and installing required tools online...${NC}"
            echo ""

            apt update
            apt install -y wget curl openssl tar net-tools

            if [ $? -ne 0 ]; then
                show_error "Failed to install required tools. Please check your internet connection."
                exit 1
            fi

            show_success "System preparation completed successfully."
            sleep 2
        else
            # Offline installation script
            set -euo pipefail

            # مسیر فایل فشرده
            ARCHIVE="/root/backhaul_pro.tar.gz"
            TARGET_DIR="/usr/local/backhaul_offline"

            # مرحله ۱: همیشه استخراج کن
            echo ">>> Extracting $ARCHIVE ..."
            sudo rm -rf "$TARGET_DIR"      # اگر چیزی قبلاً بود، پاک کن
            sudo mkdir -p "$TARGET_DIR"
            sudo tar -xzf "$ARCHIVE" -C /usr/local/
            echo "✔ Extracted to $TARGET_DIR"

            # مرحله ۲: تشخیص نسخه اوبونتو و معماری
            UBUNTU_VER=$(lsb_release -sr)     # مثلا 22.04
            ARCH=$(dpkg --print-architecture) # مثلا amd64 یا arm64

            REPO_PATH="$TARGET_DIR/ubuntu${UBUNTU_VER}-${ARCH}"

            if [ ! -d "$REPO_PATH" ]; then
              echo "❌ No matching repo found for Ubuntu $UBUNTU_VER [$ARCH]"
              echo "Available repos:"
              ls -1 "$TARGET_DIR"
              sudo rm -rf "$TARGET_DIR"
              exit 1
            fi

            echo ">>> Using repo: $REPO_PATH"

            # مرحله ۳: معرفی مخزن به apt
            LIST_FILE="/etc/apt/sources.list.d/offline-local.list"
            echo "deb [trusted=yes] file:$REPO_PATH ./" | sudo tee "$LIST_FILE"

            # مرحله ۴: بروزرسانی و نصب بسته‌ها
            sudo apt update
            sudo apt install -y wget curl openssl tar net-tools

            # Copy backhaul_pro from TARGET_DIR
            BACKHAUL_DIR="/root/backhaul_pro"
            sudo mkdir -p "$BACKHAUL_DIR"
            if [ -f "$TARGET_DIR/backhaul_pro" ]; then
                sudo cp "$TARGET_DIR/backhaul_pro" "$BACKHAUL_DIR/backhaul_pro"
                echo "✔ Copied backhaul_pro to $BACKHAUL_DIR"
            else
                echo "⚠ backhaul_pro not found in $TARGET_DIR"
            fi

            # مرحله ۵: پاک‌سازی پوشه استخراج‌شده
            echo ">>> Cleaning up..."
            sudo rm -rf "$TARGET_DIR"
            echo "✔ Removed $TARGET_DIR"

            echo "✅ Offline installation finished successfully."
            sleep 2
        fi

        # Early check for existing config.toml
        clear_screen "Config Check" "Configuration File Check"

        CONFIG_NUMBER=1
        NEW_CONFIG_CREATED=false
        SERVICE_TO_RESTART=""
        REPLACE_AND_DELETE_SERVICE=false

        if [ -f "$CONFIG_FILE" ]; then
            show_warning "Configuration file '$CONFIG_FILE' already exists."
            show_prompt
            echo -e "${WHITE}Please choose an option:${NC}"
            echo -e "${GREEN}1) Replace existing file${NC}"
            echo -e "${GREEN}2) Create new config file with automatic numbering${NC}"
            echo ""
            
            while true; do
                read -p "$(echo -e "${YELLOW}Enter your choice (1/2): ${NC}")" config_file_choice
                
                case $config_file_choice in
                    1)
                        show_warning "Replacing the existing configuration file..."
                        CONFIG_NUMBER=1
                        
                        # Find the existing service that uses the main config file
                        SERVICE_FILE_PATH=$(grep -lr --fixed-strings "$CONFIG_FILE" /etc/systemd/system/ | grep -E 'backhaul(_pro)?[0-9]*\.service|utunnel[0-9]*\.service' | head -n 1)
                        if [ -n "$SERVICE_FILE_PATH" ] && [ -f "$SERVICE_FILE_PATH" ]; then
                            SERVICE_TO_RESTART=$(basename "$SERVICE_FILE_PATH")
                            show_info "Existing service found: $SERVICE_TO_RESTART"
                            read -p "$(echo -e "${YELLOW}Do you want to stop and delete this service? (y/N): ${NC}")" delete_service_choice
                            if [[ "$delete_service_choice" =~ ^[Yy]$ ]]; then
                                uninstall_service "$SERVICE_TO_RESTART"
                                REPLACE_AND_DELETE_SERVICE=true
                            else
                                show_info "Will restart existing service: $SERVICE_TO_RESTART after configuration."
                            fi
                        fi
                        break
                        ;;
                    2)
                        CONFIG_NUMBER=$(find_next_config_number "config")
                        if [ $CONFIG_NUMBER -eq 1 ]; then
                            # This case should not happen if config.toml exists, but as a fallback
                            CONFIG_FILE="$BACKHAUL_DIR/config.toml"
                        else
                            CONFIG_FILE="$BACKHAUL_DIR/config${CONFIG_NUMBER}.toml"
                        fi
                        NEW_CONFIG_CREATED=true
                        show_success "New configuration file will be created as '$CONFIG_FILE'."
                        break
                        ;;
                    *)
                        show_error "Invalid choice. Please enter '1' or '2'."
                        ;;
                esac
            done
        fi

        # Download backhaul_pro.tar.gz if online
        if [ "$install_mode" = "online" ]; then
            clear_screen "1" "Downloading Backhaul Pro"
            echo -e "${WHITE}Downloading backhaul_pro.tar.gz from:${NC}"
            echo -e "${CYAN}$DOWNLOAD_URL${NC}"
            echo ""

            if command -v wget &> /dev/null; then
                wget -O /tmp/backhaul_pro.tar.gz "$DOWNLOAD_URL"
            elif command -v curl &> /dev/null; then
                curl -Lo /tmp/backhaul_pro.tar.gz "$DOWNLOAD_URL"
            else
                show_error "Neither wget nor curl is available. Please install one of them."
                exit 1
            fi

            if [ $? -ne 0 ]; then
                show_error "Failed to download backhaul_pro.tar.gz. Please check the URL and internet connection."
                exit 1
            fi

            show_success "Download completed successfully."
            sleep 2
        fi

        # Step 2: Create backhaul_pro folder
        clear_screen "2" "Folder Configuration"

        if [ -d "$BACKHAUL_DIR" ]; then
            show_info "Folder '$BACKHAUL_DIR' already exists. Using existing folder."
        else
            mkdir "$BACKHAUL_DIR"
            show_success "Folder '$BACKHAUL_DIR' created successfully."
        fi

        sleep 2

        # Extract if online
        if [ "$install_mode" = "online" ]; then
            clear_screen "3" "Extracting Backhaul Pro"
            echo -e "${WHITE}Extracting backhaul_pro.tar.gz to:${NC}"
            echo -e "${CYAN}$BACKHAUL_DIR${NC}"
            echo ""

            tar -xzf /tmp/backhaul_pro.tar.gz -C "$BACKHAUL_DIR"

            if [ $? -ne 0 ]; then
                show_error "Failed to extract backhaul_pro.tar.gz. The file may be corrupted."
                exit 1
            fi

            show_success "Extraction completed successfully."

            # Clean up temporary file
            rm -f /tmp/backhaul_pro.tar.gz

            sleep 2
        fi

        # Step 4: Ask for server or client selection
        clear_screen "4" "Configuration Type Selection"
        show_prompt
        echo -e "${WHITE}Please select the configuration type:${NC}"
        echo -e "${GREEN}1) Server ${CYAN}(IRAN)${NC}"
        echo -e "${GREEN}2) Client ${YELLOW}(KHAREJ)${NC}"
        echo ""

        while true; do
            read -p "$(echo -e "${YELLOW}Enter your choice (1/2): ${NC}")" config_type
            
            case $config_type in
                1)
                    CONFIG_TYPE="server"
                    show_info "Server configuration selected (IRAN)."
                    ;;
                2)
                    CONFIG_TYPE="client"
                    show_info "Client configuration selected (KHAREJ)."
                    ;;
                *)
                    show_error "Invalid choice. Please enter '1' for server or '2' for client."
                    continue
                    ;;
            esac
            break
        done

        # Step 5: Get configuration variables
        clear_screen "5" "${CONFIG_TYPE^} Configuration Settings ($([ "$CONFIG_TYPE" = "server" ] && echo "IRAN" || echo "KHAREJ"))"

        if [ "$CONFIG_TYPE" = "client" ]; then
            # Step 5.1: Server IP (only for client)
            show_prompt
            echo -e "${WHITE}Server IP Configuration${NC}"
            while true; do
                read -p "$(echo -e "${YELLOW}Enter server IP address: ${NC}")" SERVER_IP
                if [ -z "$SERVER_IP" ]; then
                    show_error "Server IP cannot be empty. Please enter a valid IP address."
                    continue
                fi
                if ! validate_ip "$SERVER_IP"; then
                    show_error "Invalid IP format. Please enter a valid IPv4 address."
                    continue
                fi
                break
            done
            show_success "Server IP set to: $SERVER_IP"
            echo ""
        fi

        # Step 5.2: Tunnel port
        show_prompt
        echo -e "${WHITE}Tunnel Port Configuration${NC}"
        while true; do
            read -p "$(echo -e "${YELLOW}Enter tunnel port (default: 3080): ${NC}")" TUNNEL_PORT
            TUNNEL_PORT=${TUNNEL_PORT:-3080}
            
            if [[ ! "$TUNNEL_PORT" =~ ^[0-9]+$ ]] || [ "$TUNNEL_PORT" -lt 1 ] || [ "$TUNNEL_PORT" -gt 65535 ]; then
                show_error "Invalid port number. Please enter a number between 1-65535."
                continue
            fi
            if ! check_port_available "$TUNNEL_PORT"; then
                show_error "Port $TUNNEL_PORT is already in use. Please choose another."
                continue
            fi
            break
        done
        show_success "Tunnel port set to: $TUNNEL_PORT"
        echo ""

        # Step 5.3: Protocol selection
        show_prompt
        echo -e "${WHITE}Protocol Selection${NC}"
        echo -e "${CYAN}Select protocol:${NC}"
        echo -e "${GREEN}1) udp${NC}"
        echo -e "${GREEN}2) tcp${NC}"
        echo -e "${GREEN}3) tcpmux${NC}"
        echo -e "${GREEN}4) ws${NC}"
        echo -e "${GREEN}5) wss${NC}"
        echo -e "${GREEN}6) wsmux${NC}"
        echo -e "${GREEN}7) wssmux${NC}"
        echo ""

        while true; do
            read -p "$(echo -e "${YELLOW}Enter protocol choice (1-7, default: 2): ${NC}")" protocol_choice
            protocol_choice=${protocol_choice:-2}
            
            case $protocol_choice in
                1) PROTOCOL="udp"; break ;;
                2) PROTOCOL="tcp"; break ;;
                3) PROTOCOL="tcpmux"; break ;;
                4) PROTOCOL="ws"; break ;;
                5) PROTOCOL="wss"; break ;;
                6) PROTOCOL="wsmux"; break ;;
                7) PROTOCOL="wssmux"; break ;;
                *) show_error "Invalid choice. Please enter a number between 1-7." ;;
            esac
        done
        show_success "Protocol set to: $PROTOCOL"
        echo ""

        # Step 5.4: Token selection
        show_prompt
        echo -e "${WHITE}Token Configuration${NC}"
        echo -e "${CYAN}Token generation options:${NC}"
        echo -e "${GREEN}1) Generate random 32-bit token (default)${NC}"
        echo -e "${GREEN}2) Enter custom token${NC}"
        echo ""

        while true; do
            read -p "$(echo -e "${YELLOW}Enter your choice (1/2, default: 1): ${NC}")" token_choice
            token_choice=${token_choice:-1}
            
            case $token_choice in
                1)
                    RANDOM_TOKEN=$(openssl rand -hex 16)
                    show_success "Random token generated: $RANDOM_TOKEN"
                    break
                    ;;
                2)
                    while true; do
                        read -p "$(echo -e "${YELLOW}Enter custom token: ${NC}")" RANDOM_TOKEN
                        if [ -z "$RANDOM_TOKEN" ]; then
                            show_error "Token cannot be empty. Please enter a token."
                            continue
                        fi
                        break
                    done
                    show_success "Custom token set."
                    break
                    ;;
                *)
                    show_error "Invalid choice. Please enter '1' or '2'."
                    ;;
            esac
        done
        echo ""

        # Step 5.5: Web port
        show_prompt
        echo -e "${WHITE}Web Port Configuration${NC}"
        while true; do
            read -p "$(echo -e "${YELLOW}Enter web port (default: 2060): ${NC}")" WEB_PORT
            WEB_PORT=${WEB_PORT:-2060}
            
            if [[ ! "$WEB_PORT" =~ ^[0-9]+$ ]] || [ "$WEB_PORT" -lt 1 ] || [ "$WEB_PORT" -gt 65535 ]; then
                show_error "Invalid port number. Please enter a number between 1-65535."
                continue
            fi
            if ! check_port_available "$WEB_PORT"; then
                show_error "Port $WEB_PORT is already in use. Please choose another."
                continue
            fi
            break
        done
        show_success "Web port set to: $WEB_PORT"
        echo ""

        if [ "$CONFIG_TYPE" = "server" ]; then
            # Step 5.6: Ports configuration (only for server)
            show_prompt
            echo -e "${WHITE}Ports Configuration${NC}"
            echo -e "${CYAN}Enter port entries (at least one is required)${NC}"
            echo -e "${YELLOW}Format examples: 443-600, 443-600:5201, 443-600=1.1.1.1:5201, 443, 4000=5000, 127.0.0.2:443=5201, etc.${NC}"
            echo -e "${YELLOW}Press Enter without any value to finish${NC}"
            echo ""
            ports=()
            port_count=1
            
            while true; do
                read -p "$(echo -e "${YELLOW}Port entry $port_count: ${NC}")" port_input
                if [ -z "$port_input" ]; then
                    if [ ${#ports[@]} -eq 0 ]; then
                        show_error "At least one port entry is required. Please enter at least one."
                        continue
                    else
                        break
                    fi
                fi
                
                # Validate entry
                if ! validate_port_entry "$port_input"; then
                    show_error "Invalid port entry format or values. Please check examples and ensure ports are 1-65535 and IPs are valid."
                    continue
                fi
                
                # Check local ports availability
                local_ports=$(extract_local_ports "$port_input")
                port_in_use=false
                for p in $local_ports; do
                    if ! check_port_available "$p"; then
                        show_error "Local port $p in entry '$port_input' is already in use."
                        port_in_use=true
                        break
                    fi
                done
                if $port_in_use; then
                    continue
                fi
                
                ports+=("$port_input")
                show_success "Port entry $port_count added: $port_input"
                ((port_count++))
            done
            echo ""
        fi

        # Step 6: Create config.toml file
        clear_screen "6" "Creating Configuration File"

        # Create the config.toml file
        if [ "$CONFIG_TYPE" = "server" ]; then
            cat > "$CONFIG_FILE" << EOF
[server]
bind_addr = "0.0.0.0:$TUNNEL_PORT"
transport = "$PROTOCOL"
token = "$RANDOM_TOKEN"
web_port = $WEB_PORT
ports = [
EOF

            # Add ports to the array
            for port_entry in "${ports[@]}"; do
                echo "\"$port_entry\"," >> "$CONFIG_FILE"
            done
            
            # Remove trailing comma if present and close array
            if [ ${#ports[@]} -gt 0 ]; then
                sed -i '$ s/,$//' "$CONFIG_FILE"
            fi
            echo "]" >> "$CONFIG_FILE"
        else
            cat > "$CONFIG_FILE" << EOF
[client]
remote_addr = "$SERVER_IP:$TUNNEL_PORT"
transport = "$PROTOCOL"
token = "$RANDOM_TOKEN"
web_port = $WEB_PORT
EOF
        fi

        show_success "${CONFIG_TYPE^} configuration file '$CONFIG_FILE' created successfully."
        if [ "$CONFIG_TYPE" = "server" ]; then
            show_info "Number of port entries added: ${#ports[@]}"
        fi

        sleep 2

        # Step 7: Set execute permission for backhaul_pro binary
        clear_screen "7" "Setting Execute Permissions"
        if [ -f "$BACKHAUL_DIR/backhaul_pro" ]; then
            chmod +x "$BACKHAUL_DIR/backhaul_pro"
            show_success "Execute permission set for backhaul_pro binary."
        else
            show_warning "backhaul_pro binary not found in $BACKHAUL_DIR"
            show_info "Please check if the extraction was successful."
        fi

        sleep 2

        # Step 8: Create systemd service file
        clear_screen "8" "Creating Systemd Service"

        # Determine service name based on config number
        if [ $CONFIG_NUMBER -eq 1 ]; then
            if [ -n "$SERVICE_TO_RESTART" ] && ! $REPLACE_AND_DELETE_SERVICE; then
                 SERVICE_FILE="/etc/systemd/system/$SERVICE_TO_RESTART"
            else
                 SERVICE_FILE="/etc/systemd/system/backhaul_pro.service"
            fi
        else
            SERVICE_FILE="/etc/systemd/system/backhaul_pro${CONFIG_NUMBER}.service"
        fi

        sudo tee "$SERVICE_FILE" > /dev/null << EOL
[Unit]
Description=Backhaul Pro (Auto Tunning) Reverse Tunnel Service - Config ${CONFIG_NUMBER}
After=network.target

[Service]
Type=simple
ExecStart=$BACKHAUL_DIR/backhaul_pro -c $CONFIG_FILE
Restart=always
RestartSec=3
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
EOL

        show_success "Systemd service file created successfully: $(basename "$SERVICE_FILE")"

        sleep 2

        # Step 9: Start and enable the service
        clear_screen "9" "Starting Backhaul Pro Service"

        echo -e "${WHITE}Reloading systemd daemon...${NC}"
        sudo systemctl daemon-reload
        show_success "Systemd daemon reloaded."

        echo -e "${WHITE}Enabling service to start on boot...${NC}"
        sudo systemctl enable "$(basename "$SERVICE_FILE")"
        show_success "Service enabled to start on boot."

        if [ -n "$SERVICE_TO_RESTART" ] && ! $REPLACE_AND_DELETE_SERVICE; then
            echo -e "${WHITE}Restarting service...${NC}"
            sudo systemctl restart "$(basename "$SERVICE_FILE")"
            show_success "Service restarted."
        else
            echo -e "${WHITE}Starting service...${NC}"
            sudo systemctl start "$(basename "$SERVICE_FILE")"
            show_success "Service started."
        fi

        echo -e "${WHITE}Waiting for service to initialize...${NC}"
        sleep 3

        echo -e "${WHITE}Checking service status...${NC}"
        sudo systemctl status "$(basename "$SERVICE_FILE")"

        # Install bh-p
        install_bh_p

        # Final summary
        clear_screen "COMPLETED" "Backhaul Pro Installation Summary"
        echo -e "${GREEN}╔══════════════════════════════════════════════════════════╗${NC}"
        echo -e "${GREEN}║                  INSTALLATION COMPLETED                  ║${NC}"
        echo -e "${GREEN}╚══════════════════════════════════════════════════════════╝${NC}"
        echo ""
        echo -e "${CYAN}Configuration Summary:${NC}"
        echo -e "${WHITE}• Installation directory: ${CYAN}$BACKHAUL_DIR${NC}"
        echo -e "${WHITE}• Configuration file: ${CYAN}$CONFIG_FILE${NC}"
        echo -e "${WHITE}• Service file: ${CYAN}$(basename "$SERVICE_FILE")${NC}"
        echo -e "${WHITE}• Configuration type: ${CYAN}${CONFIG_TYPE^} ($([ "$CONFIG_TYPE" = "server" ] && echo "IRAN" || echo "KHAREJ"))${NC}"
        echo ""
        echo -e "${YELLOW}Service Management Commands:${NC}"
        echo -e "${GREEN}  Check status: ${WHITE}sudo systemctl status $(basename "$SERVICE_FILE")${NC}"
        echo -e "${GREEN}  Start service: ${WHITE}sudo systemctl start $(basename "$SERVICE_FILE")${NC}"
        echo -e "${GREEN}  Stop service: ${WHITE}sudo systemctl stop $(basename "$SERVICE_FILE")${NC}"
        echo -e "${GREEN}  Restart service: ${WHITE}sudo systemctl restart $(basename "$SERVICE_FILE")${NC}"
        echo -e "${GREEN}  View logs: ${WHITE}sudo journalctl -u $(basename "$SERVICE_FILE") -f${NC}"
        echo ""
        if [ $CONFIG_NUMBER -gt 1 ]; then
            echo -e "${YELLOW}Note: This is configuration number $CONFIG_NUMBER${NC}"
            echo -e "${YELLOW}You can manage multiple configurations simultaneously.${NC}"
            echo ""
        fi
        echo -e "${YELLOW}For central management: ${WHITE}bh-p${NC}"
        echo ""
        echo -e "${BLUE}Thank you for using Backhaul Pro!${NC}"
        ;;
esac
