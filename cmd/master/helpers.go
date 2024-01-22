package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"net"
	"os"
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

func pingServer(srv *eimasterlib.EIServerInfo, timeout time.Duration) {
	conn, err := net.DialUDP("udp", nil, &srv.Addr)
	if err != nil {
		log.Errorf("net.DialUDP failed: %s", err)
		return
	}
	defer conn.Close()

	startTime := time.Now()
	conn.SetDeadline(startTime.Add(timeout))

	msg := []byte{3, 0, 0, 0}
	for i := 0; i < 5; i++ {
		n, err := conn.Write(msg)
		if n < len(msg) || err != nil {
			log.Errorf("conn.Write failed: %s. %d bytes were read", err, n)
			return
		}
	}

	ping := 0
	buf := make([]byte, 256)
	n, _ := conn.Read(buf)
	if n > 0 {
		ping = int(time.Since(startTime).Milliseconds())
		if ping == 0 {
			ping = 1
		} else if ping < 0 {
			ping = 0
		}
	}
	srv.Ping = ping
}

func pingServers(servers []eimasterlib.EIServerInfo) {
	var wg sync.WaitGroup
	for i := range servers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			pingServer(&servers[i], 2500*time.Millisecond)
		}(i)
	}
	wg.Wait()
}

func saveDataToGOB(path string, data interface{}) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := gob.NewEncoder(f)
	return encoder.Encode(data)
}

func loadDataFromGOB(path string, data interface{}) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	decoder := gob.NewDecoder(f)
	return decoder.Decode(data)
}
