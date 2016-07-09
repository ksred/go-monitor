package main

import (
	"bytes"
	"errors"
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

	"github.com/patrickmn/go-cache"
	"gopkg.in/yaml.v2"
)

// All these values should be in lowercase in the yaml file
// See go-monitor.yaml.sample
type ymlConf struct {
	Processes []string
	Config    struct {
		MessageBirdToken  string
		MessageBirdSender string
		Recipients        string
		DefaultTTL        time.Duration
		ServerNiceName    string
	}
}

func main() {
	//fmt.Println("Go Monitor running")
	// Load config
	conf := ymlConf{}
	data, err := ioutil.ReadFile("/usr/local/etc/go-monitor.yml")
	if err != nil {
		log.Fatal(err)
	}
	err = parseConf(data, &conf)
	if err != nil {
		log.Fatal(err)
	}

	recipientNumber := conf.Config.Recipients
	processes := conf.Processes

	// Default notification fromt config
	// Refresh time is 60 seconds
	c := cache.New(conf.Config.DefaultTTL*time.Minute, 60*time.Second)
	procErrChan := make(chan string, len(processes))

	server, err := getServerInfo(&conf)
	if err != nil {
		//fmt.Println("Error getting server information, using NIL")
		server = "NIL"
	}

	// Parent waitgroup for the two go functions below
	var wgParent sync.WaitGroup
	wgParent.Add(2)

	// One go func for the adding of procs to error channel
	go func() {
		var wg sync.WaitGroup
		for {
			wg.Add(len(processes))

			for index, proc := range processes {
				go checkProcess(proc, procErrChan, &wg)

				// Sleep when the loop is done
				// This is how often the checks for each process will run
				if index == len(processes)-1 {
					// Check every 60 seconds
					time.Sleep(60 * time.Second)
				}
			}
			wg.Wait()
		}
	}()

	// Another go func for reading the results from the error chan
	go func() {
		for {
			procErr := <-procErrChan
			go notifyProcError(procErr, server, recipientNumber, c, &conf)
		}
	}()

	// Never die
	wgParent.Wait()
}

func parseConf(data []byte, conf *ymlConf) (err error) {
	yaml.Unmarshal(data, conf)

	// Do validation checks
	if len(conf.Processes) < 1 {
		return errors.New("Config: We need to monitor at least one process")
	}
	if conf.Config.MessageBirdToken == "" {
		return errors.New("Config: MessageBird token not set")
	}
	if conf.Config.MessageBirdSender == "" {
		return errors.New("Config: MessageBird sender not set")
	}
	if conf.Config.Recipients == "" {
		return errors.New("Config: Recipients list is empty")
	}
	if conf.Config.DefaultTTL == 0 {
		return errors.New("Config: DefaultTTL empty")
	}
	if conf.Config.ServerNiceName == "" {
		return errors.New("Config: ServerNiceName empty")
	}

	return
}

func getServerInfo(conf *ymlConf) (server string, err error) {
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

	server = conf.Config.ServerNiceName + " " + host + " with IP " + ip.String()
	return server, nil
}

func checkProcess(processName string, procErrChan chan string, wg *sync.WaitGroup) {
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

	//fmt.Println(&b2)
	lines, err := lineCounter(&b2)
	if err != nil {
		//fmt.Printf("Error: %s\n", err.Error())
		os.Exit(0)
	}

	// Mark this task as done
	// Error if done after passing data to channels
	wg.Done()

	if lines == 0 {
		//fmt.Printf("Error: no process %s found running!\n", processName)
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
func notifyProcError(proc string, server string, recipientNumber string, c *cache.Cache, conf *ymlConf) {
	if len(proc) > 0 {
		//fmt.Printf("### ERROR: proc %s not running!\n", proc)

		// Check cache for process
		_, found := c.Get(proc)
		if found {
			// Wait until expiry before another notification
			//fmt.Printf("Process %s stored in cache, skipping\n", proc)
			return
		}

		// If proc not in cache, store in cache
		c.Set(proc, true, cache.DefaultExpiration)

		// Send text message
		authToken := conf.Config.MessageBirdToken
		urlStr := "https://rest.messagebird.com/messages"

		v := url.Values{}
		v.Set("recipients", recipientNumber)
		v.Set("originator", conf.Config.MessageBirdSender)
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
			//fmt.Printf("Error: %s\n", err.Error())
			return
		}
		//fmt.Println(resp.Status)

		//fmt.Println("Notification sent!")
	}
}
