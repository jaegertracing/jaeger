// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

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
