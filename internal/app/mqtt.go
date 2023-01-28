package app

import (
	"fmt"
	proto "github.com/huin/mqtt"
	"github.com/jeffallen/mqtt"
	"net"
	"sync/atomic"
	"time"
)

func InitMqtt() {
	go func() {
		for {
			worker()

			time.Sleep(time.Second * 1)
		}
	}()
}

var conn *mqtt.ClientConn
var online int32 = 0

func GetMqttHost() string {
	if Args["mqtt"] != "" {
		return Args["mqtt"]
	}
	return "192.168.1.5:1883"
}

func worker() {
	c, err := net.Dial("tcp", GetMqttHost())
	if err != nil {
		fmt.Printf("mqtt connect fails: %s\n", err)
		return
	}

	conn = mqtt.NewClientConn(c)
	if err = conn.Connect("", ""); err != nil {
		fmt.Printf("mqtt connect fails: %s\n", err)
		return
	}

	tqs := []proto.TopicQos{proto.TopicQos{Topic: "homeassistant/status"}}
	conn.Subscribe(tqs)
	atomic.StoreInt32(&online, 1)

	for m := range conn.Incoming {
		fmt.Printf("mqtt recv: %s\n", m.TopicName)
		InitDeviceTracker()
	}

	atomic.StoreInt32(&online, 0)
}

func PublishOnline(topic string, data string, retain bool) {
	go func() {
		a := 0
		for a < 60 {
			if !Publish(topic, data, retain) {
				time.Sleep(time.Second * 1)
			} else {
				break
			}
			a = a + 1
		}
	}()
}

func Publish(topic string, data string, retain bool) bool {
	if atomic.LoadInt32(&online) == 0 {
		return false
	}

	payload := []byte(data)

	msg := &proto.Publish{
		Header:    proto.Header{Retain: retain},
		TopicName: topic,
		Payload:   proto.BytesPayload(payload),
	}
	conn.Publish(msg)
	return true
}
