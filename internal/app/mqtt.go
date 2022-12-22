package app

import (
	"fmt"
	proto "github.com/huin/mqtt"
	"github.com/jeffallen/mqtt"
	"net"
	"time"
)

func InitMqtt() {
	go func() {
		for {
			worker()

			time.Sleep(time.Second * 10)
		}
	}()
}

var conn *mqtt.ClientConn
var online bool

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

	online = true

	for m := range conn.Incoming {
		fmt.Printf("mqtt recv: %s\n", m.TopicName)
	}

	online = false
}

func Publish(topic string, data string, retain bool) {
	if !online {
		return
	}

	payload := []byte(data)

	msg := &proto.Publish{
		Header:    proto.Header{Retain: retain},
		TopicName: topic,
		Payload:   proto.BytesPayload(payload),
	}
	conn.Publish(msg)
}
