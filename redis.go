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
	"strconv"
	"strings"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/gliderlabs/logspout/router"
)

var (
	AllowedTypelist = []string{"generic"}
)

const (
	MISSING_FIELDS_MESSAGE   = "Missing required fields for logtype"
	MANDATORY_FIELDS_GENERIC = "level, threadid, file, line"
)

type RedisAdapter struct {
	route         *router.Route
	pool          *redis.Pool
	key           string
	docker_host   string
	use_v0        bool
	logstash_type string
}

type GenericItems struct {
	Level    string `json:"level"`
	Threadid string `json:"threadid"`
	File     string `json:"file"`
	Line     int    `json:"line"`
}

type GenericFields struct {
	Docker      DockerFields `json:"docker"`
	Logtype     string       `json:"log_type"`
	Generic     GenericItems `json:generic`
	Instance    string       `json:"instance"`
	Role        string       `json:"role"`
	Application string       `json:"application"`
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
	Docker      DockerFields `json:"docker"`
	Decodeerror string       `json:"decode_error`
}

type LogstashMessageV0 struct {
	Type       string         `json:"@type,omitempty"`
	Timestamp  string         `json:"@timestamp"`
	Sourcehost string         `json:"@source_host"`
	Message    string         `json:"@message"`
	Fields     LogstashFields `json:"@fields"`
}

type LogstashMessageV1 struct {
	Type        string       `json:"@type,omitempty"`
	Timestamp   string       `json:"@timestamp"`
	Sourcehost  string       `json:"host"`
	Message     string       `json:"message"`
	Decodeerror string       `json:"decode_error`
	Fields      DockerFields `json:"docker"`
}

type LogstashMessageGeneric struct {
	Timestamp  string        `json:"@timestamp"`
	Sourcehost string        `json:"@source_host"`
	Message    string        `json:"@message"`
	Fields     GenericFields `json:"@fields"`
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
	debug := getopt(route.Options, "debug", "DEBUG", "") != ""

	database_s := getopt(route.Options, "database", "REDIS_DATABASE", "0")
	database, err := strconv.Atoi(database_s)
	if err != nil {
		return nil, errorf("Invalid Redis database number specified: %s. Please verify & fix", database_s)
	}

	if debug {
		log.Printf("Using Redis server '%s', dbnum: %d, password?: %t, pushkey: '%s', v0 layout?: %t, logstash type: '%s'\n",
			address, database, password != "", key, use_v0, logstash_type)
	}

	pool := newRedisConnectionPool(address, password, database)

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
	}, nil
}

func (a *RedisAdapter) Stream(logstream chan *router.Message) {
	conn := a.pool.Get()
	defer conn.Close()

	mute := false

	for m := range logstream {

		var js []byte
		var err error
		// if Data passes all checks, logstashMessageGeneric is an non-empty struct
		// Possibly an error message is provided with the reason why the provided json is not accepted
		logstashMessageGeneric, decodeError := validJsonMessage(m.Data)
		if logstashMessageGeneric == (LogstashMessageGeneric{}) {
			js, err = createLogstashMessage(m, a.docker_host, a.use_v0, a.logstash_type, decodeError)
			if err != nil {
				if !mute {
					log.Println("redis: error on json.Marshal (muting until recovered): ", err)
					mute = true
				}
				continue
			}
		} else {
			// merge Docker fields into provided json
			js, _ = json.Marshal(mergedWithdockerFields(m, logstashMessageGeneric, a.docker_host))
		}
		log.Printf("Json: %s", js)
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

func newRedisConnectionPool(server, password string, database int) *redis.Pool {
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

func createLogstashMessage(m *router.Message, docker_host string, use_v0 bool, logstash_type string, decodeError string) ([]byte, error) {
	image, image_tag := splitImage(m.Container.Config.Image)
	cid := m.Container.ID[0:12]
	name := m.Container.Name[1:]
	timestamp := m.Time.UTC().Format(time.RFC3339Nano)

	var msg interface{}

	if use_v0 {
		msg = LogstashMessageV0{
			Type:       logstash_type,
			Message:    m.Data,
			Timestamp:  timestamp,
			Sourcehost: m.Container.Config.Hostname,
			Fields: LogstashFields{
				Docker: DockerFields{
					CID:        cid,
					Name:       name,
					Image:      image,
					ImageTag:   image_tag,
					Source:     m.Source,
					DockerHost: docker_host,
				},
				Decodeerror: decodeError,
			},
		}
	} else {
		msg = LogstashMessageV1{
			Type:        logstash_type,
			Message:     m.Data,
			Timestamp:   timestamp,
			Sourcehost:  m.Container.Config.Hostname,
			Decodeerror: decodeError,
			Fields: DockerFields{
				CID:        cid,
				Name:       name,
				Image:      image,
				ImageTag:   image_tag,
				Source:     m.Source,
				DockerHost: docker_host,
			},
		}
	}

	return json.Marshal(msg)

}

func mergedWithdockerFields(m *router.Message, obj LogstashMessageGeneric, docker_host string) LogstashMessageGeneric {
	image, image_tag := splitImage(m.Container.Config.Image)
	cid := m.Container.ID[0:12]
	name := m.Container.Name[1:]

	local := obj
	local.Fields.Docker.Name = name
	local.Fields.Docker.CID = cid
	local.Fields.Docker.Image = image
	local.Fields.Docker.ImageTag = image_tag
	local.Fields.Docker.DockerHost = docker_host

	return local
}

func validJsonMessage(s string) (LogstashMessageGeneric, string) {
	var msg LogstashMessageGeneric
	err := json.Unmarshal([]byte(s), &msg)
	if err != nil {
		// Unmarshalling not possible, message not valid as json
		return LogstashMessageGeneric{}, ""
	}
	if msg.Timestamp == "" ||
		msg.Sourcehost == "" ||
		msg.Message == "" {
		// logtype in json is an unsupported logtype
		return LogstashMessageGeneric{}, ""
	}
	if !contains(AllowedTypelist, msg.Fields.Logtype) {
		// logtype in json is an unsupported logtype
		return LogstashMessageGeneric{}, ""
	}
	if msg.Fields.Generic.Level == "" ||
		msg.Fields.Generic.Threadid == "" ||
		msg.Fields.Generic.File == "" ||
		msg.Fields.Generic.Line == 0 {
		return LogstashMessageGeneric{}, fmt.Sprintf("%s %s (%s)", MISSING_FIELDS_MESSAGE, msg.Fields.Logtype, MANDATORY_FIELDS_GENERIC)
	}

	return msg, ""
}

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}
