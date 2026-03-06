package io

import (
	"bufio"
	"net"
)

// BufferedConn wraps a net.Conn with a buffered reader, ensuring that
// multiple json.Decoder instances can safely read from the same connection
// without losing data to their own greedy internal buffering.
type BufferedConn struct {
	conn       net.Conn
	buffReader *bufio.Reader
}

// NewBufferedConn wraps a connection with buffering.
func NewBufferedConn(conn net.Conn) *BufferedConn {
	return &BufferedConn{
		conn:       conn,
		buffReader: bufio.NewReader(conn),
	}
}

// Read reads from the buffered reader.
func (bc *BufferedConn) Read(p []byte) (int, error) {
	return bc.buffReader.Read(p)
}

// Write writes directly to the underlying connection (unbuffered).
func (bc *BufferedConn) Write(p []byte) (int, error) {
	return bc.conn.Write(p)
}

// Close closes the underlying connection.
func (bc *BufferedConn) Close() error {
	return bc.conn.Close()
}
