package servers

import "io"

// Server is the interface for servers that receive inbound span submissions from client.
type Server interface {
	Serve()
	IsServing() bool
	Stop()
	DataChan() chan *ReadBuf
	DataRecd(*ReadBuf) // must be called by consumer after reading data from the ReadBuf
}

// ReadBuf is a structure that holds the bytes to read into as well as the number of bytes
// that was read. The slice is typically pre-allocated to the max packet size and the buffers
// themselves are polled to avoid memory allocations for every new inbound message.
type ReadBuf struct {
	bytes []byte
	n     int
}

// GetBytes returns the contents of the Readbuf as bytes
func (r *ReadBuf) GetBytes() []byte {
	return r.bytes[:r.n]
}

func (r *ReadBuf) Read(p []byte) (int, error) {
	if r.n == 0 {
		return 0, io.EOF
	}
	n := r.n
	copied := copy(p, r.bytes[:n])
	r.n -= copied
	return n, nil
}
