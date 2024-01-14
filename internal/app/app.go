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

var Args = map[string]string{}
var Devices = map[string]int64{}
var Hosts = []string{}
var mu sync.Mutex

func Init() {
	SetupCfg()
	Fork()
	InitArp()
	InitMqtt()
	AwayTimer()
	for {
		ArpMonitor()
		time.Sleep(time.Second * 30)
	}
}

func SetupCfg() {
	// init command arguments
	for _, key := range os.Args[1:] {
		var value string
		if i := strings.IndexByte(key, '='); i > 0 {
			key, value = key[:i], key[i+1:]
		}
		Args[key] = value
	}

	// setup devices
	if v, ok := Args["target"]; ok {
		for _, d := range strings.Split(v, ",") {
			if !strings.Contains(d, ":") {
				continue
			}
			Devices[d] = -1
		}
	}

	// setup hosts
	if v, ok := Args["host"]; ok {
		for i, d := range strings.Split(v, ",") {
			Hosts[i] = d
		}
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
	_, ok := Devices[mac]
	return ok
}

func GetObjectId(mac string) string {
	return strings.ReplaceAll(mac, ":", "")
}

func OnDoorOpen() {
	go func() {
		for i := 0; i < 10; i++ {
			for _, mac := range Hosts {
				cmd := exec.Command("ping", "-c", "1", mac)
				cmd.Start()
			}
			time.Sleep(1 * time.Second)
		}
	}()
}

func InitArp() {
	out, err := os.ReadFile("/proc/net/arp")
	if err != nil {
		fmt.Printf("ark -a fails: %s\n", err)
		os.Exit(1)
	} else {
		arp := string(out)
		for _, d := range strings.Split(arp, "\n") {
			entry := strings.Split(d, " ")
			if len(entry) > 3 {
				mac := entry[3]
				if !IsTargetDevice(mac) {
					continue
				}
				Devices[mac] = 0
				fmt.Printf("device present: %s\n", mac)
			}
		}
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
		awayInterval := GetAwayInterval()
		for scanner.Scan() {
			entry := strings.Split(scanner.Text(), " ")

			if len(entry) < 4 {
				fmt.Printf("%s", entry)
				continue
			}

			mac := entry[len(entry)-3]
			reachable := (entry[len(entry)-2] == "REACHABLE")

			if !IsTargetDevice(mac) {
				//fmt.Printf("%s", mac)
				continue
			}
			if _, ok := Devices[mac]; !ok {
				CreateDeviceTracker(mac)
			}
			mu.Lock()
			if reachable {
				Devices[mac] = time.Now().Unix() + awayInterval
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

func OnArpChanged(mac string, deleted bool, awayInterval int64) {
	mu.Lock()
	defer mu.Unlock()

	if !IsTargetDevice(mac) {
		return
	}
	if deleted {
		fmt.Printf("%s %t\n", mac, deleted)
		Devices[mac] = time.Now().Unix() + awayInterval
	} else {
		old := Devices[mac]
		Devices[mac] = 0
		if old == -1 {
			Publish(GetObjectId(mac)+"/state", "home", true)
		}
		fmt.Printf("device present: %s\n", mac)
	}
}

func AwayTimer() {
	timer1 := time.NewTicker(10 * time.Second)
	go func(t *time.Ticker) {
		for {
			<-t.C
			fmt.Println("Away timer!")
			OnTimer()
		}
	}(timer1)
}

func OnTimer() {
	mu.Lock()
	defer mu.Unlock()

	unix := time.Now().Unix()
	for mac, expire := range Devices {
		if expire > 0 && unix > expire {
			fmt.Println("device away: ", mac)
			Devices[mac] = -1
			Publish(GetObjectId(mac)+"/state", "not_home", true)
		}
	}
}

func InitDeviceTracker() {
	mu.Lock()
	defer mu.Unlock()

	for mac, _ := range Devices {
		CreateDeviceTracker(mac)
	}

	for mac, v := range Devices {
		if v == -1 {
			Publish(GetObjectId(mac)+"/state", "not_home", true)
		} else {
			Publish(GetObjectId(mac)+"/state", "home", true)
		}
	}
}

func CreateDeviceTracker(mac string) {
	objectId := GetObjectId(mac)
	topic := fmt.Sprintf("homeassistant/device_tracker/%s/config", objectId)
	body := fmt.Sprintf(`{"state_topic": "%s/state", "unique_id":"arp_%s", "name": "Device Tracker %s", "payload_home": "home", "payload_not_home": "not_home"}`, objectId, objectId, mac)
	fmt.Printf("create tracker %s, %s\n", topic, body)
	Publish(topic, body, false)
}
