package main

import (
	"encoding/binary"
	"io"
	"net"
	"strings"
)

// buildMsg encodes a framed message without requiring an active connection.
func buildMsg(msgType uint16, payload []byte) []byte {
	buf := make([]byte, 6+len(payload))
	binary.BigEndian.PutUint16(buf[0:2], msgType)
	binary.BigEndian.PutUint32(buf[2:6], uint32(len(payload)))
	copy(buf[6:], payload)
	return buf
}

// sendCommand writes a framed message on the active connection.
func sendCommand(msgType uint16, payload []byte) {
	if activeConn == nil {
		return
	}
	activeConn.Write(buildMsg(msgType, payload))
}

// recvMsg reads one framed message from conn.
// Returns (0, nil, err) on read failure.
func recvMsg(conn net.Conn) (uint16, []byte, error) {
	header := make([]byte, 6)
	if _, err := io.ReadFull(conn, header); err != nil {
		return 0, nil, err
	}

	msgType := binary.BigEndian.Uint16(header[0:2])
	length := binary.BigEndian.Uint32(header[2:6])

	if length == 0 {
		return msgType, []byte{}, nil
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(conn, payload); err != nil {
		return 0, nil, err
	}

	return msgType, payload, nil
}

// isNetworkError returns true for transient connectivity drops.
// Returns false for genuine application / logic errors.
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, kw := range []string{
		"connection reset", "broken pipe", "use of closed",
		"forcibly closed", "connection refused", "i/o timeout", "timed out",
	} {
		if strings.Contains(msg, kw) {
			return true
		}
	}
	return false
}
