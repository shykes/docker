package beam

import (
	"fmt"
	"testing"
	"time"
	"io"
)

func setTimeout(t *testing.T) *time.Timer {
	return time.AfterFunc(1 * time.Second, func() { t.Fatal("timeout")})
}

func TestHubBackendThenClient(t *testing.T) {
	timer := setTimeout(t)
	defer timer.Stop()
	hub, err := Hub()
	if err != nil {
		t.Fatal(err)
	}
	defer hub.Close()
	backendExposed := make(chan struct{})
	backendFinished := make(chan struct{})
	go func() {
		defer close(backendFinished)
		fmt.Printf("sending expose endpoint to the hub\n")
		endpoint, err := SendPipe(hub, []byte("expose foo"))
		if err != nil {
			t.Fatal(err)
		}
		close(backendExposed)
		fmt.Printf("waiting for connections through the endpoint\n")
		data, _, err := Receive(endpoint)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Printf("received connection through the endpoint: %s\n", data)
		if string(data) != "foo bar" {
			t.Fatalf("unexpected data: '%s'", string(data))
		}
	}()
	fmt.Printf("Waiting for backend to be exposed first\n")
	<-backendExposed
	fmt.Printf("Sending client message to the hub\n")
	if err := Send(hub, []byte("foo bar"), nil); err != nil {
		t.Fatal(err)
	}
	fmt.Printf("waiting for backend to complete\n")
	<-backendFinished
}

func TestHub10Clients(t *testing.T) {
	timer := setTimeout(t)
	defer timer.Stop()
	hub, err := Hub()
	if err != nil {
		t.Fatal(err)
	}
	defer hub.Close()
	go func() {
		for i:=0; i<10; i++ {
			client, err := SendPipe(hub, []byte(fmt.Sprintf("foo %d", i)))
			if err != nil {
				t.Fatal(err)
			}
		}
	}()
	// With the current implementation of Hub there is no guaranteed way to 
	time.Sleep(500 * time.Millisecond)
	defer client.Close()
	// no such backend, we expect the connection to be closed without response
	data, f, err := Receive(client)
	if err != io.EOF {
		t.Fatalf("unexpected error: %v", err)
	}
	if f != nil {
		t.Fatalf("unexpected attachment\n", f)
	}
	if data != nil {
		t.Fatalf("unexpected payload %v\n", data)
	}
}
