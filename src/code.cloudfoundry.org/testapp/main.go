package main

import (
	"bytes"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

func generateBasicServer(response string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(response + " meow"))
	}

}

func runHTTPSEndpoint() {
	server := generateBasicServer("encrypted")
	http.HandleFunc("/https", server)
	err := http.ListenAndServeTLS(":8082", "/var/vcap/jobs/testapp/config/certs/server.crt", "/var/vcap/jobs/testapp/config/certs/private.key", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

func runHTTPEndpoint() {
	server := generateBasicServer("unencrypted")
	http.HandleFunc("/http", server)
	err := http.ListenAndServe(":8081", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

func runTCPEndpoint() {
	listener, err := net.Listen("tcp", ":8083")
	if err != nil {
		fmt.Println("Error listening:", err.Error())
		os.Exit(1)
	}
	// Close the listener when the application closes.
	defer listener.Close()
	for {
		// Listen for an incoming connection.
		conn, err := listener.Accept()
		if err != nil {
			log.Fatal("Error accepting: ", err.Error())
		}
		// Handle connections in a new goroutine.
		go handleTCPRequest(conn)
	}
}

func handleTCPRequest(conn net.Conn) {
	// Close the connection when you're done with it.
	defer conn.Close()

	// Make a buffer to hold incoming data.
	buff := make([]byte, 1024)
	// Continue to receive the data forever...
	for {
		// Wait for a message
		_, err := conn.Read(buff)
		if err != nil {
			fmt.Printf("Closing tcp connection: %s\n", err.Error())
			return
		}

		// Respond
		var writeBuffer bytes.Buffer
		writeBuffer.Write([]byte("tcp meow"))
		_, err = conn.Write(writeBuffer.Bytes())
		if err != nil {
			fmt.Printf("Closing tcp connection: %s\n", err.Error())
			return
		}
	}
}

func main() {
	go runHTTPSEndpoint()
	go runHTTPEndpoint()
	go runTCPEndpoint()
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	<-sigs
}
