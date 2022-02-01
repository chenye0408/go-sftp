package main

import (
	"bufio"
	"fmt"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
)

func main() {
	//fill in your credential and IP address
	user := "username"
	password := "password"
	host := "127.0.0.1"
	port := 22
	proxyAddress := "http://example_proxy"

	//set up credential
	var auths = make([]ssh.AuthMethod, 0)
	auths = append(auths, ssh.Password(password))

	//add public key auth if needed
	signer, err := ssh.ParsePrivateKey([]byte(privateKey))
	if err != nil {
		panic(err)
	}
	auths = append(auths, ssh.PublicKeys(signer))

	config := ssh.ClientConfig{
		User:              user,
		Auth:              auths,
		Timeout:           30,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Config:            ssh.Config{
			KeyExchanges:   []string{"diffie-hellman-group1-sha1"}, //this KeyExchanges may differ from different sftp server config
		},
	}

	//connect to sftp server through a proxy
	dstAddr := fmt.Sprintf("%s:%d", host, port)
	proxyUrl, err := url.Parse(proxyAddress)
	if err != nil {
		fmt.Println("parse porxy url error: ", err)
		os.Exit(1)
	}
	conn, err := tunneledSSHClient(proxyUrl, dstAddr, &config)
	if err != nil {
		fmt.Println("connect error, ", err)
		os.Exit(1)
	}
	defer conn.Close()

	//new sftp client
	client, err := sftp.NewClient(conn)
	if err != nil {
		fmt.Println("new sftp client error, ", err)
		os.Exit(1)
	}
	defer client.Close()

	//open remote file
	srcFile, err :=  client.OpenFile("/home/out/example_file", os.O_RDONLY)
	if err != nil {
		fmt.Println("open remote file error, ", err)
		os.Exit(1)
	}
	defer srcFile.Close()

	//create local file
	dstFile, err := os.Create("/emp/example_file")
	if err != nil {
		fmt.Println("create local file error, ", err)
		os.Exit(1)
	}
	defer dstFile.Close()

	//download file to local disk
	bytes, err := io.Copy(dstFile, srcFile)
	if err != nil {
		fmt.Println("download file error, ", err)
		os.Exit(1)
	}
	fmt.Printf("%d bytes download\n", bytes)
}

func tunneledSSHClient(proxy *url.URL, sshServerAddress string, sshConfig *ssh.ClientConfig) (*ssh.Client, error) {
	proxyAddr := proxy.Host
	log.Printf("dialing proxy %q ...", proxyAddr)
	//dial proxy
	c, err := net.Dial("tcp", proxyAddr)
	if err != nil {
		return nil, fmt.Errorf("dialing proxy %q failed: %v", proxyAddr, err)
	}
	//send CONNECT request to proxy to request a tunnel to remote sftp server
	_, err = fmt.Fprintf(c, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", sshServerAddress, proxy.Hostname())
	if err != nil {
		return nil, fmt.Errorf("send CONNECT request error: %v", err)
	}
	//read response
	reader := bufio.NewReader(c)
	res, err := http.ReadResponse(reader, nil)
	if err != nil {
		return nil, fmt.Errorf("reading HTTP response from CONNECT to %s via proxy %s failed: %v",
			sshServerAddress, proxyAddr, err)
	}
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("proxy error from %s while dialing %s: %v", proxyAddr, sshServerAddress, res.Status)
	}

	//establish ssh connection through the tunnel
	tunnelConn, chans, reqs, err := ssh.NewClientConn(c, sshServerAddress, sshConfig)
	if err != nil {
		return nil, err
	}

	return ssh.NewClient(tunnelConn, chans, reqs), nil
}

//set your ssh private key used to authenticate
const privateKey = ""