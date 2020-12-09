package util

import (
	"math/rand"
	"net"
	"time"
)

const (
	readBufferSize  = 1024
	writeBufferSize = 1024
)

func DelayMeasurement(network, address string, timeout time.Duration) (time.Duration, error) {
	now := time.Now()
	d := net.Dialer{Timeout: timeout}
	conn, err := d.Dial(network, address)
	delay := time.Since(now)
	if err != nil {
		return delay, err
	}

	conn.Close()

	return delay, nil
}

func BandwidthMeasurementClient(conn net.Conn, bytesDownlink int, timeout time.Duration) (float32, float32, error) {
	timeStart := time.Now()
	var timeToFirstByte time.Duration

	if timeout > 0 {
		err := conn.SetReadDeadline(timeStart.Add(timeout))
		if err != nil {
			return 0, 0, err
		}
	}

	b := make([]byte, readBufferSize)
	for bytesRead := 0; bytesRead < bytesDownlink; {
		n := bytesDownlink - bytesRead
		if n > len(b) {
			n = len(b)
		}
		m, err := conn.Read(b[:n])
		if err != nil {
			return 0, 0, err
		}
		if bytesRead == 0 {
			timeToFirstByte = time.Since(timeStart)
		}
		bytesRead += m
	}

	timeToLastByte := time.Since(timeStart)
	bps := float32(bytesDownlink) / float32(timeToLastByte) * float32(time.Second)
	bpsRead := float32(bytesDownlink) / float32(timeToLastByte-timeToFirstByte) * float32(time.Second)

	return bps, bpsRead, nil
}

func BandwidthMeasurementServer(conn net.Conn, bytesDownlink int, timeout time.Duration) error {
	if timeout > 0 {
		err := conn.SetWriteDeadline(time.Now().Add(timeout))
		if err != nil {
			return err
		}
	}

	b := make([]byte, writeBufferSize)
	for bytesWritten := 0; bytesWritten < bytesDownlink; {
		n := bytesDownlink - bytesWritten
		if n > len(b) {
			n = len(b)
		}
		rand.Read(b[:n])
		m, err := conn.Write(b[:n])
		if err != nil {
			return err
		}
		bytesWritten += m
	}

	return nil
}
