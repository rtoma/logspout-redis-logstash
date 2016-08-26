// Based on:
// - https://github.com/looplab/logspout-logstash/blob/master/logstash.go
// - https://github.com/gettyimages/logspout-kafka/blob/master/kafka.go
// - https://github.com/gliderlabs/logspout/pull/41/files
// - https://github.com/fsouza/go-dockerclient/blob/master/container.go#L222

package redis

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/gliderlabs/logspout/router"
)

const (
	NO_MESSAGE_PROVIDED     = "no message"
	LOGTYPE_APPLICATIONLOG  = "applog"
	LOGTYPE_ACCESSLOG       = "accesslog"
	DEFAULT_CONNECT_TIMEOUT = 100
	DEFAULT_READ_TIMEOUT    = 300
	DEFAULT_WRITE_TIMEOUT   = 500
)

type RedisAdapter struct {
	route         *router.Route
	pool          *redis.Pool
	key           string
	docker_host   string
	use_v0        bool
	logstash_type string
	dedot_labels  bool
	mute_errors   bool
	msg_counter   int
}

type DockerFields struct {
	Name       string            `json:"name"`
	CID        string            `json:"cid"`
	Image      string            `json:"image"`
	ImageTag   string            `json:"image_tag,omitempty"`
	Source     string            `json:"source"`
	DockerHost string            `json:"docker_host,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
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
	Logtype    string       `json:"logtype,omitempty"`
	// Only one of the following 3 is initialized and used, depending on the incoming json:logtype
	LogtypeAccessfields map[string]interface{} `json:"accesslog,omitempty"`
	LogtypeAppfields    map[string]interface{} `json:"applog,omitempty"`
	LogtypeEventfields  map[string]interface{} `json:"event,omitempty"`
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

	// get our config keys, first from the route options (e.g. redis://<host>?opt1=val&opt1=val&...)
	// if route option is missing, attempt to get the value from the environment
	key := getopt(route.Options, "key", "REDIS_KEY", "logspout")
	password := getopt(route.Options, "password", "REDIS_PASSWORD", "")
	docker_host := getopt(route.Options, "docker_host", "REDIS_DOCKER_HOST", "")
	use_v0 := getopt(route.Options, "use_v0_layout", "REDIS_USE_V0_LAYOUT", "") != ""
	logstash_type := getopt(route.Options, "logstash_type", "REDIS_LOGSTASH_TYPE", "")
	dedot_labels := getopt(route.Options, "dedot_labels", "DEDOT_LABELS", "false") == "true"
	debug := getopt(route.Options, "debug", "DEBUG", "") != ""
	mute_errors := getopt(route.Options, "mute_errors", "MUTE_ERRORS", "true") == "true"

	connect_timeout := getintopt(route.Options, "connect_timeout", "CONNECT_TIMEOUT", DEFAULT_CONNECT_TIMEOUT)
	read_timeout := getintopt(route.Options, "read_timeout", "READ_TIMEOUT", DEFAULT_READ_TIMEOUT)
	write_timeout := getintopt(route.Options, "write_timeout", "WRITE_TIMEOUT", DEFAULT_WRITE_TIMEOUT)

	database_s := getopt(route.Options, "database", "REDIS_DATABASE", "0")
	database, err := strconv.Atoi(database_s)
	if err != nil {
		return nil, errorf("Invalid Redis database number specified: %s. Please verify & fix", database_s)
	}

	if debug {
		log.Printf("Using Redis server '%s', dbnum: %d, password?: %t, pushkey: '%s', v0 layout?: %t, logstash type: '%s'\n",
			address, database, password != "", key, use_v0, logstash_type)
        log.Printf("Dedotting docker labels: %t", dedot_labels)
		log.Printf("Timeouts set, connect: %dms, read: %dms, write: %dms\n", connect_timeout, read_timeout, write_timeout)
	}
	if connect_timeout+read_timeout+write_timeout > 950 {
		log.Printf("WARN: sum of connect, read & write timeouts > 950 ms. You risk loosing container logs as Logspout stops pumping logs after a 1.0 second timeout.")
	}

	pool := newRedisConnectionPool(address, password, database, connect_timeout, read_timeout, write_timeout)

	// lets test the water
	conn := pool.Get()
	defer conn.Close()
	res, err := conn.Do("PING")
	if err != nil {
		return nil, errorf("Cannot connect to Redis server %s: %v", address, err)
	}
	if debug {
		log.Printf("Redis connect successful, got response: %s\n", res)
	}

	return &RedisAdapter{
		route:         route,
		pool:          pool,
		key:           key,
		docker_host:   docker_host,
		use_v0:        use_v0,
		logstash_type: logstash_type,
		dedot_labels:  dedot_labels,
		mute_errors:   mute_errors,
		msg_counter:   0,
	}, nil
}

func (a *RedisAdapter) Stream(logstream chan *router.Message) {
	conn := a.pool.Get()
	defer conn.Close()

	mute := false

	for m := range logstream {
		a.msg_counter += 1
		msg_id := fmt.Sprintf("%s#%d", m.Container.ID[0:12], a.msg_counter)

		js, err := createLogstashMessage(m, a.docker_host, a.use_v0, a.logstash_type, a.dedot_labels)
		if err != nil {
			if a.mute_errors {
				if !mute {
					log.Printf("redis[%s]: error on json.Marshal (muting until recovered): %s\n", msg_id, err)
					mute = true
				}
			} else {
				log.Printf("redis[%s]: error on json.Marshal: %s\n", msg_id, err)
			}
			continue
		}
		_, err = conn.Do("RPUSH", a.key, js)
		if err != nil {
			if a.mute_errors {
				if !mute {
					log.Printf("redis[%s]: error on rpush (muting until restored): %s\n", msg_id, err)
				}
			} else {
				log.Printf("redis[%s]: error on rpush: %s\n", msg_id, err)
			}
			mute = true

			// first close old connection
			conn.Close()

			// next open new connection
			conn = a.pool.Get()

			// since message is already marshaled, send again
			_, err = conn.Do("RPUSH", a.key, js)
			if err != nil {
				conn.Close()
				if !a.mute_errors {
					log.Printf("redis[%s]: error on rpush (retry): %s\n", msg_id, err)
				}
			} else {
				log.Printf("redis[%s]: successful retry rpush after error\n", msg_id)
				mute = false
			}

			continue
		} else {
			if mute {
				log.Printf("redis[%s]: successful rpush after error\n", msg_id)
				mute = false
			}
		}
	}
}

func errorf(format string, a ...interface{}) (err error) {
	err = fmt.Errorf(format, a...)
	if os.Getenv("DEBUG") != "" {
		fmt.Println(err.Error())
	}
	return
}

func getopt(options map[string]string, optkey string, envkey string, default_value string) (value string) {
	value = options[optkey]
	if value == "" {
		value = os.Getenv(envkey)
		if value == "" {
			value = default_value
		}
	}
	return
}
func getintopt(options map[string]string, optkey string, envkey string, default_value int) (value int) {
	value_s := options[optkey]
	if value_s == "" {
		value_s = os.Getenv(envkey)
	}
	if value_s == "" {
		value = default_value
	} else {
		var err error
		value, err = strconv.Atoi(value_s)
		if err != nil {
			log.Printf("Invalid value for integer paramater %s: %s - using default: %d\n", optkey, value_s, default_value)
			value = default_value
		}
	}
	return
}

func newRedisConnectionPool(server, password string, database int, connect_timeout int, read_timeout int, write_timeout int) *redis.Pool {
	return &redis.Pool{
		MaxIdle:     1,
		IdleTimeout: 240 * time.Second,
		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial("tcp", server,
				redis.DialConnectTimeout(time.Duration(connect_timeout)*time.Millisecond),
				redis.DialReadTimeout(time.Duration(read_timeout)*time.Millisecond),
				redis.DialWriteTimeout(time.Duration(write_timeout)*time.Millisecond))
			if err != nil {
				return nil, err
			}
			if password != "" {
				if _, err := c.Do("AUTH", password); err != nil {
					c.Close()
					return nil, err
				}
			}
			if database > 0 {
				if _, err := c.Do("SELECT", database); err != nil {
					c.Close()
					return nil, err
				}
			}
			return c, nil
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			if err != nil {
				log.Println("redis: test on borrow failed: ", err)
			}
			return err
		},
	}
}

func splitImage(image_tag string) (image string, tag string) {
	colon := strings.LastIndex(image_tag, ":")
	sep := strings.LastIndex(image_tag, "/")
	if colon > -1 && sep < colon {
		image = image_tag[0:colon]
		tag = image_tag[colon+1:]
	} else {
		image = image_tag
	}
	return
}

func dedotLabels(labels map[string]string) map[string]string {
	for key, _ := range labels {
		if strings.Contains(key, ".") {
			dedotted_label := strings.Replace(key, ".", "_", -1)
			labels[dedotted_label] = labels[key]
			delete(labels, key)
		}
	}

	return labels
}

func createLogstashMessage(m *router.Message, docker_host string, use_v0 bool, logstash_type string, dedot_labels bool) ([]byte, error) {
	image, image_tag := splitImage(m.Container.Config.Image)
	cid := m.Container.ID[0:12]
	name := m.Container.Name[1:]
	timestamp := m.Time.UTC().Format(time.RFC3339Nano)

	if use_v0 {
		msg := LogstashMessageV0{}

		msg.Type = logstash_type
		msg.Timestamp = timestamp
		msg.Message = m.Data
		msg.Sourcehost = m.Container.Config.Hostname
		msg.Fields.Docker.CID = cid
		msg.Fields.Docker.Name = name
		msg.Fields.Docker.Image = image
		msg.Fields.Docker.ImageTag = image_tag
		msg.Fields.Docker.Source = m.Source
		msg.Fields.Docker.DockerHost = docker_host

		// see https://github.com/rtoma/logspout-redis-logstash/issues/11
		if dedot_labels {
			msg.Fields.Docker.Labels = dedotLabels(m.Container.Config.Labels)
		} else {
			msg.Fields.Docker.Labels = m.Container.Config.Labels
		}

		return json.Marshal(msg)
	} else {
		msg := LogstashMessageV1{}

		msg.Type = logstash_type
		msg.Timestamp = timestamp
		msg.Sourcehost = m.Container.Config.Hostname
		msg.Fields.CID = cid
		msg.Fields.Name = name
		msg.Fields.Image = image
		msg.Fields.ImageTag = image_tag
		msg.Fields.Source = m.Source
		msg.Fields.DockerHost = docker_host

		// see https://github.com/rtoma/logspout-redis-logstash/issues/11
		if dedot_labels {
			msg.Fields.Labels = dedotLabels(m.Container.Config.Labels)
		} else {
			msg.Fields.Labels = m.Container.Config.Labels
		}

		// Check if the message to log itself is json
		if validJsonMessage(strings.TrimSpace(m.Data)) {
			// So it is, include it in the LogstashmessageV1
			err := msg.UnmarshalDynamicJSON([]byte(m.Data))
			if err != nil {
				// Can't unmarshall the json (invalid?), put it in message
				msg.Message = m.Data
			} else if msg.Message == "" {
				msg.Message = NO_MESSAGE_PROVIDED
			}
		} else {
			// Regular logging (no json)
			msg.Message = m.Data
		}
		return json.Marshal(msg)
	}

}

func validJsonMessage(s string) bool {

	if !strings.HasPrefix(s, "{") || !strings.HasSuffix(s, "}") {
		return false
	}
	return true
}

func (d *LogstashMessageV1) UnmarshalDynamicJSON(data []byte) error {
	var dynMap map[string]interface{}

	if d == nil {
		return errors.New("RawString: UnmarshalJSON on nil pointer")
	}

	if err := json.Unmarshal(data, &dynMap); err != nil {
		return err
	}

	// Take logtype of the hash, but only if it is a valid logtype
	if _, ok := dynMap["logtype"].(string); ok {
		if dynMap["logtype"].(string) == LOGTYPE_APPLICATIONLOG || dynMap["logtype"].(string) == LOGTYPE_ACCESSLOG {
			d.Logtype = dynMap["logtype"].(string)
			delete(dynMap, "logtype")
		}
	}
	// Take message out of the hash
	if _, ok := dynMap["message"]; ok {
		d.Message = dynMap["message"].(string)
		delete(dynMap, "message")
	}

	// Only initialize the "used" hash in struct
	if d.Logtype == LOGTYPE_APPLICATIONLOG {
		d.LogtypeAppfields = make(map[string]interface{}, 0)
	} else if d.Logtype == LOGTYPE_ACCESSLOG {
		d.LogtypeAccessfields = make(map[string]interface{}, 0)
	} else {
		d.LogtypeEventfields = make(map[string]interface{}, 0)
	}

	// Fill the right hash based on logtype
	for key, val := range dynMap {
		if d.Logtype == LOGTYPE_APPLICATIONLOG {
			d.LogtypeAppfields[key] = val
		} else if d.Logtype == LOGTYPE_ACCESSLOG {
			d.LogtypeAccessfields[key] = val
		} else {
			d.LogtypeEventfields[key] = val
		}
	}

	return nil
}
