// Based on:
// - https://github.com/looplab/logspout-logstash/blob/master/logstash.go
// - https://github.com/gettyimages/logspout-kafka/blob/master/kafka.go
// - https://github.com/gliderlabs/logspout/pull/41/files
// - https://github.com/fsouza/go-dockerclient/blob/master/container.go#L222

package redis

import (
    "fmt"
    "strings"
    "log"
    "os"
    "time"
    "encoding/json"
    "github.com/gliderlabs/logspout/router"
    "github.com/garyburd/redigo/redis"
)


type RedisAdapter struct {
    route       *router.Route
    pool        *redis.Pool
    key         string
    docker_host string
}

type DockerFields struct {
    Name        string `json:"name"`
    CID         string `json:"cid"`
    Image       string `json:"image"`
    ImageTag    string `json:"image_tag"`
    Source      string `json:"source"`
    DockerHost  string `json:"docker_host"`
}

type LegacyLogstashMessage struct {
    Timestamp   string          `json:"@timestamp"`
    Sourcehost  string          `json:"source_host"`
    Message     string          `json:"message"`
    Fields      DockerFields    `json:"docker"`
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

    if os.Getenv("DEBUG") != "" {
        log.Printf("Using Redis server '%s', password: %t, pushkey: '%s'\n",
                   address, password != "", key)
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
        route:          route,
        pool:           pool,
        key:            key,
        docker_host:    docker_host,
    }, nil
}

func (a *RedisAdapter) Stream(logstream chan *router.Message) {
    conn := a.pool.Get()
    defer conn.Close()

    mute := false

    for m := range logstream {

        image_name, image_tag := splitImage(m.Container.Config.Image)

        msg := LegacyLogstashMessage{
            Message:    m.Data,
            Timestamp:  m.Time.Format(time.RFC3339Nano),
            Sourcehost: m.Container.Config.Hostname,
            Fields:     DockerFields{
                CID:        m.Container.ID[0:12],    // make ID a bit shorter
                Name:       m.Container.Name[1:],    // removing tailing slash
                Image:      image_name,
                ImageTag:   image_tag,
                Source:     m.Source,
                DockerHost: a.docker_host,
            },
        }
        js, err := json.Marshal(msg)
        if err != nil {
            if !mute {
                log.Println("redis: error on json.Marshal (muting until restored):", err)
                mute = true
            }
            continue
        }
        _, err = conn.Do("RPUSH", a.key, js)
        if err != nil {
            if !mute {
                log.Println("redis: error on rpush (muting until restored):", err)
                mute = true
            }
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
        MaxIdle: 3,
        IdleTimeout: 240 * time.Second,
        Dial: func () (redis.Conn, error) {
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
