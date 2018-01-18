package ss_test

import (
	"bytes"
	"context"
	"io"
	"net"
	"sync"
	"testing"

	"math/rand"

	ss "github.com/libp2p/go-stream-security"
)

var Subtests = map[string]func(t *testing.T, at, bt ss.Transport){
	"Basic":     SubtestBasic,
	"WrongPeer": SubtestWrongPeer,
	"Stream":    SubtestStream,
}

var TestMessage = []byte("hello world!")
var TestStreamLen int64 = 1024 * 8
var TestSeed int64 = 1812

func SubtestAll(t *testing.T, at, bt ss.Transport) {
	for n, f := range Subtests {
		t.Run(n, func(t *testing.T) {
			f(t, at, bt)
		})
	}
}

func randStream() io.Reader {
	return &io.LimitedReader{
		R: rand.New(rand.NewSource(TestSeed)),
		N: TestStreamLen,
	}
}

func testWriteSustain(t *testing.T, c ss.Conn) {
	source := randStream()
	n := int64(0)
	for {
		coppied, err := io.CopyN(c, source, int64(rand.Intn(8000)))
		n += coppied

		switch err {
		case io.EOF:
			if n != TestStreamLen {
				t.Fatal("incorrect random stream length")
			}
			return
		case nil:
		default:
			t.Fatal(err)
		}
	}
}

func testReadSustain(t *testing.T, c ss.Conn) {
	expected := randStream()
	total := 0
	ebuf := make([]byte, 1024)
	abuf := make([]byte, 1024)
	for {
		n, err := c.Read(abuf)
		if err != nil {
			t.Fatal(err)
		}
		total += n
		_, err = io.ReadFull(expected, ebuf[:n])
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(abuf[:n], ebuf[:n]) {
			t.Fatal("bytes not equal")
		}
		if total == int(TestStreamLen) {
			return
		}
	}
}
func testWrite(t *testing.T, c ss.Conn) {
	n, err := c.Write(TestMessage)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(TestMessage) {
		t.Errorf("wrote %d bytes, expected to write %d bytes", n, len(TestMessage))
	}
}

func testRead(t *testing.T, c ss.Conn) {
	buf := make([]byte, 100)
	n, err := c.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(TestMessage) {
		t.Errorf("wrote %d bytes, expected to write %d bytes", n, len(TestMessage))
	}
	if !bytes.Equal(buf[:n], TestMessage) {
		t.Errorf("received bad test message: %s", string(buf[:n]))
	}
}

func testEOF(t *testing.T, c ss.Conn) {
	buf := make([]byte, 100)
	n, err := c.Read(buf)
	if n != 0 {
		t.Errorf("didn't expect to read any bytes, read: %d", n)
	}
	if err != io.EOF {
		t.Errorf("expected read to fail with EOF, got: %s", err)
	}
}

func SubtestBasic(t *testing.T, at, bt ss.Transport) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a, b := net.Pipe()

	defer a.Close()
	defer b.Close()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		c, err := at.SecureInbound(ctx, a)
		if err != nil {
			t.Fatal(err)
		}

		if c.RemotePeer() != bt.LocalPeer() {
			t.Errorf("expected peer %s, got peer %s", bt.LocalPeer(), c.RemotePeer())
		}
		testWrite(t, c)
		testRead(t, c)
		c.Close()
	}()

	go func() {
		defer wg.Done()
		c, err := bt.SecureOutbound(ctx, b, at.LocalPeer())
		if err != nil {
			t.Fatal(err)
		}
		if c.RemotePeer() != at.LocalPeer() {
			t.Errorf("expected peer %s, got peer %s", at.LocalPeer(), c.RemotePeer())
		}
		testRead(t, c)
		testWrite(t, c)
		testEOF(t, c)
		c.Close()
	}()
	wg.Wait()
}

func SubtestWrongPeer(t *testing.T, at, bt ss.Transport) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a, b := net.Pipe()

	defer a.Close()
	defer b.Close()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, err := at.SecureInbound(ctx, a)
		if err == nil {
			t.Fatal("conection should have failed")
		}
	}()

	go func() {
		defer wg.Done()
		_, err := bt.SecureOutbound(ctx, b, bt.LocalPeer())
		if err == nil {
			t.Fatal("connection should have failed")
		}
	}()
	wg.Wait()
}

func SubtestStream(t *testing.T, at, bt ss.Transport) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a, b := net.Pipe()

	defer a.Close()
	defer b.Close()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()

		c, err := at.SecureInbound(ctx, a)
		if err != nil {
			t.Fatal(err)
		}

		if c.RemotePeer() != bt.LocalPeer() {
			t.Errorf("expected peer %s, got peer %s", bt.LocalPeer(), c.RemotePeer())
		}
		var swg sync.WaitGroup
		swg.Add(2)
		go func() {
			defer swg.Done()
			testWriteSustain(t, c)
		}()
		go func() {
			defer swg.Done()
			testReadSustain(t, c)
		}()
		swg.Wait()
		c.Close()
	}()

	go func() {
		defer wg.Done()
		c, err := bt.SecureOutbound(ctx, b, at.LocalPeer())
		if err != nil {
			t.Fatal(err)
		}
		if c.RemotePeer() != at.LocalPeer() {
			t.Errorf("expected peer %s, got peer %s", at.LocalPeer(), c.RemotePeer())
		}
		io.Copy(c, c)
		c.Close()
	}()
	wg.Wait()
}