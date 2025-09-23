package tuning

import (
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
)

// GetCPUUsage returns the current CPU usage percentage.
func GetCPUUsage() (float64, error) {
	percentages, err := cpu.Percent(time.Second, false)
	if err != nil {
		return 0, err
	}
	if len(percentages) > 0 {
		return percentages[0], nil
	}
	return 0, nil
}

// GetMemoryUsage returns the current memory usage percentage.
func GetMemoryUsage() (float64, error) {
	vmStat, err := mem.VirtualMemory()
	if err != nil {
		return 0, err
	}
	return vmStat.UsedPercent, nil
}

// GetPacketLossAndThroughput returns estimated packet loss percentage and throughput (bytes/sec)
func GetPacketLossAndThroughput() (float64, float64, error) {
	counters, err := net.IOCounters(false)
	if err != nil || len(counters) == 0 {
		return 0, 0, err
	}
	// این تابع مقدار تجمعی را می‌دهد، پس باید در دو بازه زمانی فراخوانی شود و اختلاف را حساب کنیم
	// برای سادگی، یک sleep کوتاه می‌گذاریم (مثلاً 1 ثانیه)
	first := counters[0]
	time.Sleep(1 * time.Second)
	counters2, err := net.IOCounters(false)
	if err != nil || len(counters2) == 0 {
		return 0, 0, err
	}
	second := counters2[0]

	// Throughput: bytes sent + received per second
	throughput := float64((second.BytesSent + second.BytesRecv) - (first.BytesSent + first.BytesRecv))

	// Packet loss: (dropped packets) / (total packets) in this interval
	totalPackets := float64((second.PacketsSent + second.PacketsRecv) - (first.PacketsSent + first.PacketsRecv))
	lostPackets := float64((second.Dropin + second.Dropout) - (first.Dropin + first.Dropout))
	var packetLoss float64
	if totalPackets > 0 {
		packetLoss = (lostPackets / totalPackets) * 100.0
	} else {
		packetLoss = 0
	}
	return packetLoss, throughput, nil
}
