package beam

import (
	"fmt"
	"testing"
	"time"
	"io/ioutil"
	"net"
	"os"
)

func setTimeout(t *testing.T) *time.Timer {
	return time.AfterFunc(1 * time.Second, func() { t.Fatal("timeout")})
}

func testEndpoint(t *testing.T, hub *net.UnixConn, header string, count int, onReady func(), handler func([]byte, *os.File)) {
	fmt.Printf("[%s] opening endpoint\n", header)
	endpoint, err := SendPipe(hub, []byte(header))
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("[%s] endpoint is open\n", header)
	if onReady != nil {
		onReady()
	}
	defer endpoint.Close()
	for i:=0; i<count; i++ {
		fmt.Printf("[%s] waiting for connection\n", header)
		data, f, err := Receive(endpoint)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Printf("[%s] new connection: payload='%s' attachment='%v'\n", header, data, f.Fd())
		handler(data, f)
		if f != nil {
			f.Close()
		}
	}
}

func TestHubBackendThenClient(t *testing.T) {
	timer := setTimeout(t)
	defer timer.Stop()
	hub, err := Hub()
	if err != nil {
		t.Fatal(err)
	}
	defer hub.Close()
	backendReady := make(chan bool)
	go testEndpoint(t, hub, "expose foo", 1, func() { close(backendReady) }, func(data []byte, f *os.File) {
		if string(data) != "foo bar" {
			t.Fatalf("unexpected data: %s", data)
		}
		fmt.Fprintf(f, "hello there!")
		f.Sync()
	})
	<-backendReady
	testEndpoint(t, hub, "foo bar", 1, nil, func(data []byte, f *os.File) {
		if string(data) != "expose foo" {
			t.Fatalf("unexpected data: %s", data)
		}
		text, err := ioutil.ReadAll(f)
		if err != nil {
			t.Fatal(err)
		}
		if string(text) != "hello there!" {
			t.Fatalf("unexpected data: '%s'", text)
		}
	})
}

func TestClientThenBackend(t *testing.T) {
	timer := setTimeout(t)
	defer timer.Stop()
	hub, err := Hub()
	if err != nil {
		t.Fatal(err)
	}
	defer hub.Close()
	clientConnected := make(chan struct{})
	clientFinished := make(chan struct{})
	go func() {
		defer close(clientFinished)
		client, err := SendPipe(hub, []byte("foo bar"))
		if err != nil {
			t.Fatal(err)
		}
		close(clientConnected)
		endpointHeader, endpoint, err := Receive(client)
		if err != nil {
			t.Fatal(err)
		}
		// client should receive description passed by expose
		if string(endpointHeader) != "expose foo" {
			t.Fatalf("unexpected data: %s", endpointHeader)
		}
		data, err := ioutil.ReadAll(endpoint)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Printf("received data: %s\n", data)
		if string(data) != "foo indeed!" {
			t.Fatalf("unexpeted data: %s", data)
		}
	}()
	// Wait for client request to be sent
	<-clientConnected

	backendExposed := make(chan struct{})
	backendFinished := make(chan struct{})
	go func() {
		defer close(backendFinished)
		fmt.Printf("sending expose endpoint to the hub\n")
		backend, err := SendPipe(hub, []byte("expose foo"))
		if err != nil {
			t.Fatal(err)
		}
		defer backend.Close()
		close(backendExposed)
		fmt.Printf("waiting for connections through the endpoint\n")
		query, client, err := Receive(backend)
		if err != nil {
			t.Fatal(err)
		}
		if string(query) != "foo bar" {
			t.Fatalf("unexpected client query: %s", query)
		}
		fmt.Printf("received connection through the endpoint: %s\n", query)
		fmt.Fprintf(client, "foo indeed...")
		fmt.Printf("wrote data to the client\n")
		client.Sync()
		client.Close()
	}()
	fmt.Printf("Waiting for backend to be exposed\n")
	<-backendExposed
	<-backendFinished
}
