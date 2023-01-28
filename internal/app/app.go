package app

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

func Init() {
	// init command arguments
	for _, key := range os.Args[1:] {
		var value string
		if i := strings.IndexByte(key, '='); i > 0 {
			key, value = key[:i], key[i+1:]
		}
		Args[key] = value
	}

	Fork()
	InitMqtt()
	InitDeviceTracker()
	InitArp()
	AwayTimer()
	for {
		ArpMonitor()
		time.Sleep(time.Second * 30)
	}
}

func Fork() {
	if Args["d"] == "" {
		return
	}
	time.Sleep(1 * time.Second)

	if os.Getppid() != 1 {
		// I am the parent, spawn child to run as daemon
		binary, err := exec.LookPath(os.Args[0])
		if err != nil {
			fmt.Printf("Failed to lookup binary: %s\n", err)
		}
		_, err = os.StartProcess(binary, os.Args, &os.ProcAttr{Dir: "", Env: nil,
			Files: []*os.File{os.Stdin, os.Stdout, os.Stderr}, Sys: nil})
		if err != nil {
			fmt.Printf("Failed to start process: %s\n", err)
		}
		fmt.Printf("arp tracker run in background\n")
		os.Exit(0)
	} else {
		// I am the child, i.e. the daemon, start new session and detach from terminal
		_, err := syscall.Setsid()
		if err != nil {
			fmt.Printf("Failed to create new session: %s\n", err)
		}
		file, err := os.OpenFile("/dev/null", os.O_RDWR, 0)
		if err != nil {
			fmt.Printf("Failed to open /dev/null: %s\n", err)
		}
		syscall.Dup2(int(file.Fd()), int(os.Stdin.Fd()))
		syscall.Dup2(int(file.Fd()), int(os.Stdout.Fd()))
		syscall.Dup2(int(file.Fd()), int(os.Stderr.Fd()))
		file.Close()
	}
}

func IsTargetDevice(mac string) bool {
	if !strings.Contains(mac, ":") {
		return false
	}
	if Args["target"] == "all" {
		return true
	}
	return strings.Contains(Args["target"], mac)
}

func GetObjectId(mac string) string {
	return strings.ReplaceAll(mac, ":", "")
}

func InitArp() {
	out, err := exec.Command("arp", "-a").Output()
	if err != nil {
		fmt.Printf("ark -a fails: %s\n", err)
		os.Exit(1)
	} else {
		arp := string(out)
		mu.Lock()
		for _, d := range strings.Split(arp, "\n") {
			entry := strings.Split(d, " ")
			if len(entry) > 3 {
				mac := entry[3]
				if !IsTargetDevice(mac) {
					continue
				}
				Devices[mac] = 0
				CreateDeviceTracker(mac)
				Publish(GetObjectId(mac)+"/state", "home", true)
				fmt.Printf("device present: %s\n", mac)
			}
		}
		mu.Unlock()
	}
}

func GetAwayInterval() int64 {
	if Args["interval"] != "" {
		i, err := strconv.ParseInt(Args["interval"], 0, 64)
		if err != nil {
			fmt.Printf("incorrect interval: %s\n", Args["interval"])
			os.Exit(1)
		}
		fmt.Printf("away interval: %d\n", i)
		return i
	}
	return 180
}

func ArpMonitor() {
	cmd := exec.Command("ip", "monitor", "neigh")
	cmdReader, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintln(os.Stderr, "arp monitor failed", err)
		os.Exit(1)
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig: syscall.SIGTERM,
	}

	scanner := bufio.NewScanner(cmdReader)
	go func() {
		alwayInterval := GetAwayInterval()
		for scanner.Scan() {
			entry := strings.Split(scanner.Text(), " ")
			deleted := (entry[0] == "delete")

			if len(entry) < 4 {
				continue
			}

			mac := entry[4]
			if deleted {
				mac = entry[5]
			}
			if !IsTargetDevice(mac) {
				continue
			}
			if _, ok := Devices[mac]; !ok {
				CreateDeviceTracker(mac)
			}
			mu.Lock()
			if deleted {
				fmt.Printf("%s %t\n", mac, deleted)
				Devices[mac] = time.Now().Unix() + alwayInterval
			} else {
				Devices[mac] = 0
				Publish(GetObjectId(mac)+"/state", "home", true)
				fmt.Printf("device present: %s\n", mac)
			}
			mu.Unlock()
		}
	}()

	err = cmd.Start()
	if err != nil {
		fmt.Fprintln(os.Stderr, "arp monitor failed", err)
	}
	err = cmd.Wait()
	fmt.Fprintln(os.Stderr, "monitor exit", err)
}

func AwayTimer() {
	timer1 := time.NewTicker(10 * time.Second)
	go func(t *time.Ticker) {
		for {
			<-timer1.C
			fmt.Println("Away timer!")

			unix := time.Now().Unix()
			mu.Lock()
			for mac, expire := range Devices {
				if expire > 0 && unix > expire {
					fmt.Println("device away: ", mac)
					delete(Devices, mac)
					Publish(GetObjectId(mac)+"/state", "not_home", true)
				}
			}
			mu.Unlock()
		}
	}(timer1)
}

func InitDeviceTracker() {
	if v, ok := Args["target"]; ok && v != "all" {
		for _, d := range strings.Split(v, ",") {
			if !IsTargetDevice(d) {
				continue
			}
			CreateDeviceTracker(d)
		}
	}
}

func CreateDeviceTracker(mac string) {
	objectId := GetObjectId(mac)
	topic := fmt.Sprintf("homeassistant/device_tracker/%s/config", objectId)
	body := fmt.Sprintf(`{"state_topic": "%s/state", "unique_id":"arp_%s", "name": "Device Tracker %s", "payload_home": "home", "payload_not_home": "not_home"}`, objectId, objectId, mac)
	fmt.Printf("create tracker %s, %s\n", topic, body)
	PublishOnline(topic, body, false)
}

var Args = map[string]string{}
var Devices = map[string]int64{}
var mu sync.Mutex
