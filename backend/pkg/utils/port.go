package utils

import (
	"fmt"
	"net"
)

func FindAvailablePort(min, max int) int {
	for port := min; port <= max; port++ {
		addr := fmt.Sprintf(":%d", port)
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			ln.Close()
			return port
		}
	}
	panic(fmt.Sprintf("не удалось найти свободный порт в диапазоне %d-%d", min, max))
}
