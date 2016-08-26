package redis

import (
	"bytes"
	"encoding/json"
	//"log"
	"testing"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/gliderlabs/logspout/router"
	"github.com/jmoiron/jsonq"
	"github.com/stretchr/testify/assert"
)

func TestSplitImage(t *testing.T) {
	assert := assert.New(t)

	image, tag := splitImage("bla")
	assert.Equal("bla", image)
	assert.Equal("", tag)

	image, tag = splitImage("foo:latest")
	assert.Equal("foo", image)
	assert.Equal("latest", tag)

	image, tag = splitImage("foo/bar:latest")
	assert.Equal("foo/bar", image)
	assert.Equal("latest", tag)

	image, tag = splitImage("my.registry.host/some/image:1.3.4")
	assert.Equal("my.registry.host/some/image", image)
	assert.Equal("1.3.4", tag)

	image, tag = splitImage("my.registry.host:443/path/to/image:3.1.4")
	assert.Equal("my.registry.host:443/path/to/image", image)
	assert.Equal("3.1.4", tag)

	image, tag = splitImage("my.registry.host:443/path/to/image")
	assert.Equal("my.registry.host:443/path/to/image", image)
	assert.Equal("", tag)

}

func TestCreateLogstashMessageV1(t *testing.T) {

	assert := assert.New(t)

	m := router.Message{
		Container: &docker.Container{
			ID:   "6feffd9428dc",
			Name: "/my_app",
			Config: &docker.Config{
				Hostname: "container_hostname",
				Image:    "my.registry.host:443/path/to/image:1234",
				Labels:   map[string]string{"label_1": "abc", "label_2": "def"},
			},
		},
		Source: "stdout",
		Data:   "hello world",
		Time:   time.Unix(int64(1453818496), 595000000),
	}

	msg, _ := createLogstashMessage(&m, "tst-mesos-slave-001", false, "my-type", false)
	jq := makeQuery(msg)

	assert.Equal("my-type", getString(jq, "@type"))
	assert.Equal("2016-01-26T14:28:16.595Z", getString(jq, "@timestamp"))
	assert.Equal("container_hostname", getString(jq, "host"))
	assert.Equal("hello world", getString(jq, "message"))
	assert.Equal("my_app", getString(jq, "docker", "name"))
	assert.Equal("6feffd9428dc", getString(jq, "docker", "cid"))
	assert.Equal("my.registry.host:443/path/to/image", getString(jq, "docker", "image"))
	assert.Equal("1234", getString(jq, "docker", "image_tag"))
	assert.Equal("stdout", getString(jq, "docker", "source"))
	assert.Equal("tst-mesos-slave-001", getString(jq, "docker", "docker_host"))
	assert.Equal("abc", getString(jq, "docker", "labels", "label_1"))
	assert.Equal("def", getString(jq, "docker", "labels", "label_2"))

}

func TestCreateLogstashMessageV0(t *testing.T) {

	assert := assert.New(t)

	m := router.Message{
		Container: &docker.Container{
			ID:   "f00ffd9428dc",
			Name: "/my_db",
			Config: &docker.Config{
				Hostname: "container_hostname",
				Image:    "my.registry.host:443/path/to/image:4321",
				Labels:   map[string]string{"label_1": "abc", "label_2": "def"},
			},
		},
		Source: "stderr",
		Data:   "cruel world",
		Time:   time.Unix(int64(1453813310), 1000000),
	}

	msg, _ := createLogstashMessage(&m, "tst-mesos-slave-001", true, "some-type", false)
	jq := makeQuery(msg)

	assert.Equal("some-type", getString(jq, "@type"))
	assert.Equal("2016-01-26T13:01:50.001Z", getString(jq, "@timestamp"))
	assert.Equal("container_hostname", getString(jq, "@source_host"))
	assert.Equal("cruel world", getString(jq, "@message"))
	assert.Equal("my_db", getString(jq, "@fields", "docker", "name"))
	assert.Equal("f00ffd9428dc", getString(jq, "@fields", "docker", "cid"))
	assert.Equal("my.registry.host:443/path/to/image", getString(jq, "@fields", "docker", "image"))
	assert.Equal("4321", getString(jq, "@fields", "docker", "image_tag"))
	assert.Equal("stderr", getString(jq, "@fields", "docker", "source"))
	assert.Equal("tst-mesos-slave-001", getString(jq, "@fields", "docker", "docker_host"))
	assert.Equal("", getString(jq, "@fields", "decode_error"))
	assert.Equal("abc", getString(jq, "@fields", "docker", "labels", "label_1"))
	assert.Equal("def", getString(jq, "@fields", "docker", "labels", "label_2"))

}

func TestCreateLogstashMessageOptionalType(t *testing.T) {

	assert := assert.New(t)

	m := router.Message{
		Container: &docker.Container{
			ID:   "f00ffd9428dc",
			Name: "/my_db",
			Config: &docker.Config{
				Hostname: "container_hostname",
				Image:    "my.registry.host:443/path/to/image:4321",
			},
		},
		Source: "stderr",
		Data:   "cruel world",
		Time:   time.Unix(int64(1453813330), 0),
	}

	msg, _ := createLogstashMessage(&m, "tst-mesos-slave-001", true, "", false)
	jq := makeQuery(msg)
	//log.Printf("Standard message: %s", msg)

	assert.Equal("", getString(jq, "@type"))

}

func TestCreateLogstashMessageWithJsonData(t *testing.T) {

	assert := assert.New(t)

	m := router.Message{
		Container: &docker.Container{
			ID:   "6feffd9428dc",
			Name: "/my_app",
			Config: &docker.Config{
				Hostname: "container_hostname",
				Image:    "my.registry.host:443/path/to/image:1234",
			},
		},
		Source: "stdout",
		Data:   `{"logtype": "applog", "message":"something happened", "level": "DEBUG", "file": "debug.go", "line": 42}`,
		Time:   time.Unix(int64(1453818496), 595000000),
	}

	msg, _ := createLogstashMessage(&m, "tst-mesos-slave-001", false, "my-type", false)
	jq := makeQuery(msg)

	assert.Equal("something happened", getString(jq, "message"))
	assert.Equal("applog", getString(jq, "logtype"))
	assert.Equal("DEBUG", getString(jq, "applog", "level"))
	assert.Equal("debug.go", getString(jq, "applog", "file"))
	assert.Equal(42, getInt(jq, "applog", "line"))

}

func TestCreateLogstashMessageWithJsonDataAndNoMessage(t *testing.T) {

	assert := assert.New(t)

	m := router.Message{
		Container: &docker.Container{
			ID:   "6feffd9428dc",
			Name: "/my_app",
			Config: &docker.Config{
				Hostname: "container_hostname",
				Image:    "my.registry.host:443/path/to/image:1234",
			},
		},
		Source: "stdout",
		Data:   `{ "logtype": "applog", "level": "DEBUG", "file": "debug.go", "line": 14}`,
		Time:   time.Unix(int64(1453818496), 595000000),
	}

	msg, _ := createLogstashMessage(&m, "tst-mesos-slave-001", false, "my-type", false)
	jq := makeQuery(msg)

	assert.Equal("no message", getString(jq, "message"))

}

func TestCreateLogstashMessageWithJsonDataAndNoLogtype(t *testing.T) {

	assert := assert.New(t)

	m := router.Message{
		Container: &docker.Container{
			ID:   "6feffd9428dc",
			Name: "/my_app",
			Config: &docker.Config{
				Hostname: "container_hostname",
				Image:    "my.registry.host:443/path/to/image:1234",
			},
		},
		Source: "stdout",
		Data:   `{ "message":"here i am!", "level": "DEBUG", "file": "debug.go", "line": 42}`,
		Time:   time.Unix(int64(1453818496), 595000000),
	}

	msg, _ := createLogstashMessage(&m, "tst-mesos-slave-001", false, "my-type", false)
	jq := makeQuery(msg)

	assert.Equal("here i am!", getString(jq, "message"))
	assert.Equal("", getString(jq, "logtype"))
	assert.Equal("DEBUG", getString(jq, "event", "level"))
	assert.Equal("debug.go", getString(jq, "event", "file"))
	assert.Equal(42, getInt(jq, "event", "line"))

}

func TestCreateLogstashMessageWithJsonDataAndUnknownLogtype(t *testing.T) {

	assert := assert.New(t)

	m := router.Message{
		Container: &docker.Container{
			ID:   "6feffd9428dc",
			Name: "/my_app",
			Config: &docker.Config{
				Hostname: "container_hostname",
				Image:    "my.registry.host:443/path/to/image:1234",
			},
		},
		Source: "stdout",
		Data:   `{ "logtype": "nolog", "message":"here i am again!", "level": "INFO", "file": "bla.go", "line": 24}`,
		Time:   time.Unix(int64(1453818496), 595000000),
	}

	msg, _ := createLogstashMessage(&m, "tst-mesos-slave-001", false, "my-type", false)
	jq := makeQuery(msg)
	//log.Printf("Dynamic message: %s", msg)

	assert.Equal("here i am again!", getString(jq, "message"))
	assert.Equal("", getString(jq, "logtype"))
	assert.Equal("nolog", getString(jq, "event", "logtype"))
	assert.Equal("INFO", getString(jq, "event", "level"))
	assert.Equal("bla.go", getString(jq, "event", "file"))
	assert.Equal(24, getInt(jq, "event", "line"))

}

func TestCreateLogstashMessageWithJsonDataAndAccesLogtype(t *testing.T) {

	assert := assert.New(t)

	m := router.Message{
		Container: &docker.Container{
			ID:   "6feffd9428dc",
			Name: "/my_app",
			Config: &docker.Config{
				Hostname: "container_hostname",
				Image:    "my.registry.host:443/path/to/image:1234",
			},
		},
		Source: "stdout",
		Data:   `{"agent":"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_10_5) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/49.0.2623.87 Safari/537.36","auth":"-","bytes":3488,"client":"[::1]:50393","cookies":"JSESSIONID=eqsjg19bvla01dst8smi6d0f; bol.workbench.remember=emicklei; dev_appserver_login=test@example.com:False:185804764220139124118; _ga=GA1.1.1754835192.1422042636; ","httpversion":"HTTP/1.1","ident":"-","jsession_id":"","logtype":"accesslog","message":"/internal/apidocs.json/v1/policies","mime":"application/json","referrer":"http://localhost:9191/internal/apidocs/","response":200,"site":"localhost:9191","ssl":"false","time_in_sec":0,"time_in_usec":613,"unique_id":"","verb":"GET"}`,
		Time:   time.Unix(int64(1453818496), 595000000),
	}

	msg, _ := createLogstashMessage(&m, "tst-mesos-slave-001", false, "my-type", false)
	jq := makeQuery(msg)
	//log.Printf("Dynamic message: %s", msg)

	assert.Equal("accesslog", getString(jq, "logtype"))
	assert.Equal("/internal/apidocs.json/v1/policies", getString(jq, "message"))
	assert.Equal(200, getInt(jq, "accesslog", "response"))
	assert.Equal(3488, getInt(jq, "accesslog", "bytes"))

}

func TestCreateLogstashMessageWithJsonDataAndNumericLogtype(t *testing.T) {

	assert := assert.New(t)

	m := router.Message{
		Container: &docker.Container{
			ID:   "6feffd9428dc",
			Name: "/my_app",
			Config: &docker.Config{
				Hostname: "container_hostname",
				Image:    "my.registry.host:443/path/to/image:1234",
			},
		},
		Source: "stdout",
		Data:   `{ "logtype": 1, "message":"here i am!", "level": "DEBUG", "file": "debug.go", "line": 42}`,
		Time:   time.Unix(int64(1453818496), 595000000),
	}

	msg, _ := createLogstashMessage(&m, "tst-mesos-slave-001", false, "my-type", false)
	jq := makeQuery(msg)
	//log.Printf("Dynamic message: %s", msg)

	assert.Equal("", getString(jq, "logtype"))
	assert.Equal(42, getInt(jq, "event", "line"))

}

func TestCreateLogstashMessageWithInvalidJsonData(t *testing.T) {

	assert := assert.New(t)

	m := router.Message{
		Container: &docker.Container{
			ID:   "6feffd9428dc",
			Name: "/my_app",
			Config: &docker.Config{
				Hostname: "container_hostname",
				Image:    "my.registry.host:443/path/to/image:1234",
			},
		},
		Source: "stdout",
		Data:   `{ "logtype": 1, "message":"here i am!", ": "DEBUG", "file": "debug.go", "line": 42}`,
		Time:   time.Unix(int64(1453818496), 595000000),
	}

	msg, _ := createLogstashMessage(&m, "tst-mesos-slave-001", false, "my-type", false)
	jq := makeQuery(msg)
	//log.Printf("Dynamic message invalid json: %s", msg)

	assert.Equal("", getString(jq, "logtype"))

}

func TestCreateLogstashMessageV0WithDeDottingEnabled(t *testing.T) {

	assert := assert.New(t)

	m := router.Message{
		Container: &docker.Container{
			ID:   "f00ffd9428dc",
			Name: "/my_db",
			Config: &docker.Config{
				Hostname: "container_hostname",
				Image:    "my.registry.host:443/path/to/image:4321",
				Labels:   map[string]string{"label.1.2.3": "abc", "label.3.2.1": "def"},
			},
		},
		Source: "stderr",
		Data:   "cruel world",
		Time:   time.Unix(int64(1453813310), 1000000),
	}

	msg, _ := createLogstashMessage(&m, "tst-mesos-slave-001", true, "some-type", true)
	jq := makeQuery(msg)
	//log.Printf("%s", msg)

	assert.Equal("abc", getString(jq, "@fields", "docker", "labels", "label_1_2_3"))
	assert.Equal("def", getString(jq, "@fields", "docker", "labels", "label_3_2_1"))

}

func TestCreateLogstashMessageV1WithDeDottingEnabled(t *testing.T) {

	assert := assert.New(t)

	m := router.Message{
		Container: &docker.Container{
			ID:   "f00ffd9428dc",
			Name: "/my_db",
			Config: &docker.Config{
				Hostname: "container_hostname",
				Image:    "my.registry.host:443/path/to/image:4321",
				Labels:   map[string]string{"label.1.2.3": "abc", "label.3.2.1": "def"},
			},
		},
		Source: "stderr",
		Data:   "cruel world",
		Time:   time.Unix(int64(1453813310), 1000000),
	}

	msg, _ := createLogstashMessage(&m, "tst-mesos-slave-001", false, "some-type", true)
	jq := makeQuery(msg)

	assert.Equal("abc", getString(jq, "docker", "labels", "label_1_2_3"))
	assert.Equal("def", getString(jq, "docker", "labels", "label_3_2_1"))

}

func TestCreateLogstashMessageV0WithDeDottingDefaultDisabled(t *testing.T) {

	assert := assert.New(t)

	m := router.Message{
		Container: &docker.Container{
			ID:   "f00ffd9428dc",
			Name: "/my_db",
			Config: &docker.Config{
				Hostname: "container_hostname",
				Image:    "my.registry.host:443/path/to/image:4321",
				Labels:   map[string]string{"label.1.2.3": "abc", "label.3.2.1": "def"},
			},
		},
		Source: "stderr",
		Data:   "cruel world",
		Time:   time.Unix(int64(1453813310), 1000000),
	}

	msg, _ := createLogstashMessage(&m, "tst-mesos-slave-001", true, "some-type", false)
	jq := makeQuery(msg)

	assert.Equal("abc", getString(jq, "@fields", "docker", "labels", "label.1.2.3"))
	assert.Equal("def", getString(jq, "@fields", "docker", "labels", "label.3.2.1"))

}

func TestCreateLogstashMessageV1WithDeDottingDefaultDisabled(t *testing.T) {

	assert := assert.New(t)

	m := router.Message{
		Container: &docker.Container{
			ID:   "f00ffd9428dc",
			Name: "/my_db",
			Config: &docker.Config{
				Hostname: "container_hostname",
				Image:    "my.registry.host:443/path/to/image:4321",
				Labels:   map[string]string{"label.1.2.3": "abc", "label.3.2.1": "def"},
			},
		},
		Source: "stderr",
		Data:   "cruel world",
		Time:   time.Unix(int64(1453813310), 1000000),
	}

	msg, _ := createLogstashMessage(&m, "tst-mesos-slave-001", false, "some-type", false)
	jq := makeQuery(msg)
    //log.Printf("%s", msg)

	assert.Equal("abc", getString(jq, "docker", "labels", "label.1.2.3"))
	assert.Equal("def", getString(jq, "docker", "labels", "label.3.2.1"))

}

func TestValidJsonMessageNoJson(t *testing.T) {
	assert := assert.New(t)

	js := `whateverthefuckever`
	assert.False(validJsonMessage(js))

}

func TestValidJsonMessageJson(t *testing.T) {
	assert := assert.New(t)

	js := `{"message":"foo"}`
	assert.True(validJsonMessage(js))

}

func getInt(j *jsonq.JsonQuery, s ...string) int {
	v, _ := j.Int(s...)
	return v
}

func getString(j *jsonq.JsonQuery, s ...string) string {
	v, _ := j.String(s...)
	return v
}

func makeQuery(msg []byte) *jsonq.JsonQuery {
	data := map[string]interface{}{}
	dec := json.NewDecoder(bytes.NewReader(msg))
	dec.Decode(&data)
	jq := jsonq.NewQuery(data)
	return jq
}
