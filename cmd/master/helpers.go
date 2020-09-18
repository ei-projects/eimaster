package main

import (
	"bytes"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/ei-projects/eimaster/pkg/eimasterlib"
)

func encodeASCII(src []uint8) string {
	var buf bytes.Buffer
	for _, b := range src {
		if b < 32 || b > 126 {
			buf.WriteRune('.')
		} else {
			buf.WriteByte(b)
		}
	}
	return buf.String()
}

func getHexDump(bytes []byte) string {
	result := ""
	offset := 0
	for offset < len(bytes) {
		chunkLen := len(bytes) - offset
		if chunkLen > 16 {
			chunkLen = 16
		}
		chunk := ""
		for i := 0; i < chunkLen; i++ {
			if i > 0 && i%8 == 0 {
				chunk += " "
			}
			chunk += fmt.Sprintf("%02X ", bytes[offset+i])
		}
		result += fmt.Sprintf("%08X  %-49s |%s|\n", offset,
			chunk, encodeASCII(bytes[offset:offset+chunkLen]))
		offset += chunkLen
	}
	result += fmt.Sprintf("%08X\n", offset)
	return result
}

func pingServer(srv *eimasterlib.EIServerInfo, timeoutMS time.Duration) {
	conn, err := net.DialUDP("udp", nil, &srv.Addr)
	if err != nil {
		log.Errorf("net.DialUDP failed: %s", err)
		return
	}

	startTime := time.Now()
	conn.SetDeadline(startTime.Add(timeoutMS * time.Millisecond))

	msg := []byte{3, 0, 0, 0}
	n, err := conn.Write(msg)
	if n < len(msg) || err != nil {
		log.Errorf("conn.Write failed: %s. %d bytes were read", err, n)
		return
	}

	buf := make([]byte, 256)
	n, err = conn.Read(buf)
	if n > 0 {
		srv.Ping = int(time.Now().Sub(startTime).Milliseconds())
		if srv.Ping == 0 {
			srv.Ping = 1
		}
	} else {
		srv.Ping = 0
	}
}

func pingServers(servers []eimasterlib.EIServerInfo) {
	var wg sync.WaitGroup
	for i := range servers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			pingServer(&servers[i], 500)
		}(i)
	}
	wg.Wait()
}
