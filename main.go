package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"github.com/patrickmn/go-cache"
	"gopkg.in/yaml.v2"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// All these values should be in lowercase in the yaml file
// See sample.go-monitor.yaml
type Monitor struct {
	Processes []string
	Config    struct {
		MessageBirdToken      string
		MessageBirdSender     string
		Recipients            string
		DefaultTTLSeconds     time.Duration
		ServerNiceName        string
		CheckFrequencySeconds time.Duration
	}
	writeToConsole bool
}

func main() {

	defaultConfigfile := "/usr/local/etc/go-monitor.yml"
	configFile := flag.String("f", defaultConfigfile, fmt.Sprintf("config file path, default = %s", defaultConfigfile))
	writeToConsole := flag.Bool("o", false, fmt.Sprintf("output, if true will write to console"))
	flag.Parse()

	monitor, err := createMonitorFromFile(*configFile)
	monitor.writeToConsole = *writeToConsole

	monitor.Println("Go Monitor running")

	// Default notification from config
	// Refresh time is 60 seconds
	c := cache.New(monitor.Config.DefaultTTLSeconds*time.Second, 60*time.Second)
	procErrChan := make(chan string, len(monitor.Processes))

	server, err := monitor.getServerInfo()
	if err != nil {
		monitor.Println("Error getting server information, using NIL")
		server = "NIL"
	}

	// Parent waitgroup for the two go functions below
	var wgParent sync.WaitGroup
	wgParent.Add(2)

	// One go func for the adding of procs to error channel
	go func() {
		var wg sync.WaitGroup
		for {
			wg.Add(len(monitor.Processes))

			for index, proc := range monitor.Processes {
				go monitor.checkProcess(proc, procErrChan, &wg)

				// Sleep when the loop is done
				// This is how often the checks for each process will run
				if index == len(monitor.Processes)-1 {
					// Check every 60 seconds
					time.Sleep(monitor.Config.CheckFrequencySeconds * time.Second)
				}
			}
			wg.Wait()
		}
	}()

	// Another go func for reading the results from the error chan
	go func() {
		for {
			procErr := <-procErrChan
			go monitor.notifyProcError(procErr, server, monitor.Config.Recipients, c)
		}
	}()

	// Never die
	wgParent.Wait()
}

func (monitor *Monitor) Println(message string) {

	if monitor.writeToConsole {
		fmt.Println(message)
	}
}

func (monitor *Monitor) Printf(message string, a ...interface{}) {

	if monitor.writeToConsole {
		fmt.Printf(message, a)
	}
}

func createMonitorFromFile(configFile string) (monitor *Monitor, error error) {

	data, error := ioutil.ReadFile(configFile)
	if error != nil {
		log.Fatal(error)
	}

	fmt.Println(configFile)

	yaml.Unmarshal(data, &monitor)

	error = monitor.validate()

	return
}

func (monitor *Monitor) validate() error {
	// Do validation checks
	if len(monitor.Processes) < 1 {
		return errors.New("Config: We need to monitor at least one process")
	} else {
		monitor.Printf("Processes %s\n", monitor.Processes)
	}
	if strings.Trim(monitor.Config.MessageBirdToken, " ") != "" {
		monitor.Printf("MessageBirdToken %s\n", monitor.Config.MessageBirdToken)

		if strings.Trim(monitor.Config.MessageBirdSender, "") == "" {
			return errors.New("Config: MessageBird sender not set")
		} else {
			monitor.Printf("MessageBirdSender %s\n", monitor.Config.MessageBirdSender)
		}

		if monitor.Config.Recipients == "" {
			return errors.New("Config: Recipients list is empty")
		} else {
			monitor.Printf("Recipients %s\n", monitor.Config.Recipients)
		}
	}

	if monitor.Config.DefaultTTLSeconds == 0 {
		monitor.Config.DefaultTTLSeconds = 30000
	}
	if monitor.Config.CheckFrequencySeconds == 0 {
		monitor.Config.CheckFrequencySeconds = 60
	}
	if monitor.Config.ServerNiceName == "" {
		return errors.New("Config: ServerNiceName empty")
	}

	monitor.Printf("DefaultTTLSeconds %d\n", monitor.Config.DefaultTTLSeconds)
	monitor.Printf("CheckFrequencySeconds %d\n", monitor.Config.CheckFrequencySeconds)
	monitor.Printf("ServerNiceName %v\n", monitor.Config.ServerNiceName)

	return nil
}

func (monitor *Monitor) getServerInfo() (server string, err error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	var ip net.IP
	for _, i := range ifaces {
		addrs, err := i.Addrs()
		if err != nil {
			return "", err
		}

		for _, addr := range addrs {
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
		}
	}

	// Get hostname
	host, err := os.Hostname()
	if err != nil {
		return "", err
	}

	server = monitor.Config.ServerNiceName + " " + host + " with IP " + ip.String()
	return server, nil
}

func (monitor *Monitor) checkProcess(processName string, procErrChan chan string, wg *sync.WaitGroup) {

	if strings.HasPrefix(processName, "tcp://") {
		monitor.checkTcpSocket(strings.TrimPrefix(processName, "tcp://"), procErrChan, wg)
	} else if strings.HasPrefix(processName, "http://") || strings.HasPrefix(processName, "https://") {
		monitor.checkHttpEndpoint(processName, procErrChan, wg)
	} else {
		monitor.checkLocalProcess(processName, procErrChan, wg)
	}
}

func (monitor *Monitor) checkTcpSocket(tcpAddress string, procErrChan chan string, wg *sync.WaitGroup) {
	monitor.Printf("Checking for tcp socket %s\n", tcpAddress)

	conn, err := net.Dial("tcp", tcpAddress)
	defer func() {
		if conn != nil {
			conn.Close()
		}
	}()

	if err != nil {
		monitor.Printf("Error: unable to open socket! %s\n", tcpAddress)
		procErrChan <- tcpAddress
	} else {
		monitor.Printf("Successful connection to %s \n", tcpAddress)
	}

	// Doing this keeps the channel open
	// If this is not done, the channel closes and there is a fatal error
	procErrChan <- ""
}

func (monitor *Monitor) checkHttpEndpoint(httpEndpoint string, procErrChan chan string, wg *sync.WaitGroup) {
	monitor.Printf("Checking http endpoint %s\n", httpEndpoint)

	resp, err := http.DefaultClient.Get(httpEndpoint)

	if err != nil {
		monitor.Printf("Error: unable to connect to %s - %s\n", httpEndpoint, err.Error())
		procErrChan <- httpEndpoint
	} else if resp.Status != "200 OK" {
		monitor.Printf("Error: non 200 status from %s - %s\n", httpEndpoint, resp.Status)
		procErrChan <- httpEndpoint
	} else {
		monitor.Printf("%s returns 200 OK\n", httpEndpoint, resp.Status)
	}

	// Doing this keeps the channel open
	// If this is not done, the channel closes and there is a fatal error
	procErrChan <- ""
}

func (monitor *Monitor) checkLocalProcess(processName string, procErrChan chan string, wg *sync.WaitGroup) {
	monitor.Printf("Checking for process %s\n", processName)

	c1 := exec.Command("ps", "aux")
	c2 := exec.Command("grep", processName)

	r, w := io.Pipe()
	c1.Stdout = w
	c2.Stdin = r

	var b2 bytes.Buffer
	c2.Stdout = &b2

	c1.Start()
	c2.Start()
	c1.Wait()
	w.Close()
	c2.Wait()

	//Println(&b2)
	lines, err := lineCounter(&b2)
	if err != nil {
		monitor.Printf("Error: %s\n", err.Error())
		os.Exit(0)
	}

	// Mark this task as done
	// Error if done after passing data to channels
	wg.Done()

	if lines == 0 {
		monitor.Printf("Error: no process %s found running!\n", processName)
		procErrChan <- processName
	}

	// Doing this keeps the channel open
	// If this is not done, the channel closes and there is a fatal error
	procErrChan <- ""
}

// Linecounter counts the number of lines in a given output
func lineCounter(r io.Reader) (int, error) {
	buf := make([]byte, 32*1024)
	count := 0
	lineSep := []byte{'\n'}

	for {
		c, err := r.Read(buf)
		count += bytes.Count(buf[:c], lineSep)

		switch {
		case err == io.EOF:
			return count, nil

		case err != nil:
			return count, err
		}
	}
}

// Notifyproceerror sends a notification for a given process
func (monitor *Monitor) notifyProcError(proc string, server string, recipientNumber string, c *cache.Cache) {
	if len(proc) > 0 {
		monitor.Printf("### ERROR: proc %s not running!\n", proc)

		// Check cache for process
		_, found := c.Get(proc)
		if found {
			// Wait until expiry before another notification
			monitor.Printf("Process %s stored in cache, skipping\n", proc)
			return
		}

		// If proc not in cache, store in cache
		c.Set(proc, true, cache.DefaultExpiration)

		// Send text message
		authToken := monitor.Config.MessageBirdToken
		urlStr := "https://rest.messagebird.com/messages"

		v := url.Values{}
		v.Set("recipients", recipientNumber)
		v.Set("originator", monitor.Config.MessageBirdSender)
		v.Set("body", "📢 "+proc+" not running on server "+server+"!")
		rb := *strings.NewReader(v.Encode())

		client := &http.Client{}

		req, _ := http.NewRequest("POST", urlStr, &rb)
		req.SetBasicAuth("AccessKey", authToken)
		req.Header.Add("Accept", "application/json")
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

		// Make request
		_, err := client.Do(req)
		if err != nil {
			monitor.Printf("Error: %s\n", err.Error())
			return
		}

		monitor.Println("Notification sent!")
	}
}
