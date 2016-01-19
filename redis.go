// Based on:
// - https://github.com/looplab/logspout-logstash/blob/master/logstash.go
// - https://github.com/gettyimages/logspout-kafka/blob/master/kafka.go
// - https://github.com/gliderlabs/logspout/pull/41/files
// - https://github.com/fsouza/go-dockerclient/blob/master/container.go#L222

package redis

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/gliderlabs/logspout/router"
)

type RedisAdapter struct {
	route         *router.Route
	pool          *redis.Pool
	key           string
	docker_host   string
	use_v0        bool
	logstash_type string
}

type DockerFields struct {
	Name       string `json:"name"`
	CID        string `json:"cid"`
	Image      string `json:"image"`
	ImageTag   string `json:"image_tag,omitempty"`
	Source     string `json:"source"`
	DockerHost string `json:"docker_host,omitempty"`
}

type LogstashFields struct {
	Docker DockerFields `json:"docker"`
}

type LogstashMessageV0 struct {
	Type       string         `json:"@type,omitempty"`
	Timestamp  string         `json:"@timestamp"`
	Sourcehost string         `json:"@source_host"`
	Message    string         `json:"@message"`
	Fields     LogstashFields `json:"@fields"`
}

type LogstashMessageV1 struct {
	Type       string       `json:"@type,omitempty"`
	Timestamp  string       `json:"@timestamp"`
	Sourcehost string       `json:"host"`
	Message    string       `json:"message"`
	Fields     DockerFields `json:"docker"`
}

func init() {
	router.AdapterFactories.Register(NewRedisAdapter, "redis")
}

func NewRedisAdapter(route *router.Route) (router.LogAdapter, error) {
	// add port if missing
	address := route.Address
	if !strings.Contains(address, ":") {
		address = address + ":6379"
	}

	key := route.Options["key"]
	if key == "" {
		key = getopt("REDIS_KEY", "logspout")
	}

	password := route.Options["password"]
	if password == "" {
		password = getopt("REDIS_PASSWORD", "")
	}

	docker_host := getopt("REDIS_DOCKER_HOST", "")

	use_v0 := route.Options["use_v0_layout"] != ""
	if !use_v0 {
		use_v0 = getopt("REDIS_USE_V0_LAYOUT", "") != ""
	}

	logstash_type := route.Options["logstash_type"]
	if logstash_type == "" {
		logstash_type = getopt("REDIS_LOGSTASH_TYPE", "")
	}

	if os.Getenv("DEBUG") != "" {
		log.Printf("Using Redis server '%s', password: %t, pushkey: '%s', v0 layout: %t, logstash type: '%s'\n",
			address, password != "", key, use_v0, logstash_type)
	}

	pool := newRedisConnectionPool(address, password)

	// lets test the water
	conn := pool.Get()
	defer conn.Close()
	res, err := conn.Do("PING")
	if err != nil {
		return nil, errorf("Cannot connect to Redis server %s: %v", address, err)
	}
	if os.Getenv("DEBUG") != "" {
		log.Printf("Redis connect successful, got response: %s\n", res)
	}

	return &RedisAdapter{
		route:         route,
		pool:          pool,
		key:           key,
		docker_host:   docker_host,
		use_v0:        use_v0,
		logstash_type: logstash_type,
	}, nil
}

func (a *RedisAdapter) Stream(logstream chan *router.Message) {
	conn := a.pool.Get()
	defer conn.Close()

	mute := false

	for m := range logstream {
		msg := createLogstashMessage(m, a.docker_host, a.use_v0, a.logstash_type)
		js, err := json.Marshal(msg)
		if err != nil {
			if !mute {
				log.Println("redis: error on json.Marshal (muting until recovered): ", err)
				mute = true
			}
			continue
		}
		_, err = conn.Do("RPUSH", a.key, js)
		if err != nil {
			if !mute {
				log.Println("redis: error on rpush (muting until restored): ", err)
				mute = true
			}

			// first close old connection
			conn.Close()

			// next open new connection
			conn = a.pool.Get()

			// since message is already marshaled, send again
			_, _ = conn.Do("RPUSH", a.key, js)

			continue
		}
		mute = false
	}
}

func errorf(format string, a ...interface{}) (err error) {
	err = fmt.Errorf(format, a...)
	if os.Getenv("DEBUG") != "" {
		fmt.Println(err.Error())
	}
	return
}

func getopt(name, dfault string) string {
	value := os.Getenv(name)
	if value == "" {
		value = dfault
	}
	return value
}

func newRedisConnectionPool(server, password string) *redis.Pool {
	return &redis.Pool{
		MaxIdle:     3,
		IdleTimeout: 240 * time.Second,
		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial("tcp", server)
			if err != nil {
				return nil, err
			}
			if password != "" {
				if _, err := c.Do("AUTH", password); err != nil {
					c.Close()
					return nil, err
				}
			}
			return c, err
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
	}
}

func splitImage(image string) (string, string) {
	n := strings.Index(image, ":")
	if n > -1 {
		return image[0:n], image[n+1:]
	}
	return image, ""
}

func createLogstashMessage(m *router.Message, docker_host string, use_v0 bool, logstash_type string) interface{} {
	image_name, image_tag := splitImage(m.Container.Config.Image)
	cid := m.Container.ID[0:12]
	name := m.Container.Name[1:]
	timestamp := m.Time.Format(time.RFC3339Nano)

	if use_v0 {
		return LogstashMessageV0{
			Type:       logstash_type,
			Message:    m.Data,
			Timestamp:  timestamp,
			Sourcehost: m.Container.Config.Hostname,
			Fields: LogstashFields{
				Docker: DockerFields{
					CID:        cid,
					Name:       name,
					Image:      image_name,
					ImageTag:   image_tag,
					Source:     m.Source,
					DockerHost: docker_host,
				},
			},
		}
	}

	return LogstashMessageV1{
		Type:       logstash_type,
		Message:    m.Data,
		Timestamp:  timestamp,
		Sourcehost: m.Container.Config.Hostname,
		Fields: DockerFields{
			CID:        cid,
			Name:       name,
			Image:      image_name,
			ImageTag:   image_tag,
			Source:     m.Source,
			DockerHost: docker_host,
		},
	}
}
