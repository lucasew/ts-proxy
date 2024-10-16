package tsproxy

import (
	"io"
	"sync"
)

func copyThenCancel(wg *sync.WaitGroup, rx io.Reader, tx io.Writer) {
	// defer cancel()
	io.Copy(tx, rx)
	wg.Done()
	// if flusher, ok := tx.(http.Flusher); ok {
	// 	flusher.Flush()
	// }
}

func pipeConn(a io.ReadWriter, b io.ReadWriter) {
	wg := &sync.WaitGroup{}
	wg.Add(2)
	go copyThenCancel(wg, a, b)
	go copyThenCancel(wg, b, a)
	wg.Wait()
}
