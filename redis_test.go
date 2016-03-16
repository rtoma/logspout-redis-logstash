package redis

import (
	"bytes"
	"encoding/json"
	"log"
	"strings"
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
			},
		},
		Source: "stdout",
		Data:   "hello world",
		Time:   time.Unix(int64(1453818496), 595000000),
	}

	msg, _ := createLogstashMessage(&m, "tst-mesos-slave-001", false, "my-type", "")
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
			},
		},
		Source: "stderr",
		Data:   "cruel world",
		Time:   time.Unix(int64(1453813310), 1000000),
	}

	msg, _ := createLogstashMessage(&m, "tst-mesos-slave-001", true, "some-type", "")
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
		Time:   time.Unix(int64(1453813310), 0),
	}

	msg, _ := createLogstashMessage(&m, "tst-mesos-slave-001", true, "", "")
	jq := makeQuery(msg)

	assert.Equal("", getString(jq, "@type"))

}

func TestValidJsonMessageNoJson(t *testing.T) {
	assert := assert.New(t)

	js := `whateverthefuckever`
	validJson, _ := validJsonMessage(js)
	assert.Equal(validJson, LogstashMessageGeneric{})

}

func TestValidJsonMessageIncorrectLogtype(t *testing.T) {
	assert := assert.New(t)

	js := `{
  "@source_host":"test.here.com",
  "@timestamp":"2013-10-24T09:30:46.947024155+02:00",
  "@fields":{
       "log_type": "wrong",
       "generic": {
          "level":"INFO",
          "threadid":"400004",
          "file":"file.go",
          "line":10
       },
      "instance": "001",
      "role": "kevlar-app",
      "application": "kevlar"
  },
  "@message":"hello"
}`
	validJson, _ := validJsonMessage(js)
	assert.Equal(validJson, LogstashMessageGeneric{})

}

func TestValidJsonMessageMissingGenericFields(t *testing.T) {
	assert := assert.New(t)

	js := `{
  "@source_host":"test.here.com",
  "@timestamp":"2013-10-24T09:30:46.947024155+02:00",
  "@fields":{
       "log_type": "generic",
       "generic": {
          "threadid":"400004",
          "file":"file.go",
          "line":10
       },
      "instance": "001",
      "role": "kevlar-app",
      "application": "kevlar"
  },
  "@message":"hello"
}`
	validJson, err := validJsonMessage(js)
	assert.True(strings.Contains(err, MISSING_FIELDS_MESSAGE))
	assert.Equal(validJson, LogstashMessageGeneric{})

}

func TestValidJsonMessage(t *testing.T) {
	assert := assert.New(t)

	js := `{
  "@source_host":"test.here.com",
  "@timestamp":"2013-10-24T09:30:46.947024155+02:00",
  "@fields":{
       "log_type": "generic",
       "generic": {
          "level":"INFO",
          "threadid":"400004",
          "file":"file.go",
          "line":10
       },
      "instance": "001",
      "role": "kevlar-app",
      "application": "kevlar"
  },
  "@message":"hello"
}`
	validJson, _ := validJsonMessage(js)
	//log.Printf("value: %s", validJson.Message)
	assert.NotEqual(validJson, LogstashMessageGeneric{})

}

func TestMergedWithdockerFields(t *testing.T) {
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
		Time:   time.Unix(int64(1453813310), 0),
	}

	var msg LogstashMessageGeneric
	msg = LogstashMessageGeneric{
		Message:    m.Data,
		Timestamp:  "2013-10-24T09:30:46.947024155+02:00",
		Sourcehost: m.Container.Config.Hostname,
		Fields: GenericFields{
			Logtype: "generic",
			Generic: GenericItems{
				Level:    "INFO",
				Threadid: "40004",
				File:     "my.go",
				Line:     10,
			},
			Instance:    "001",
			Role:        "kevlar-app",
			Application: "kevlar",
		},
	}

	msg = mergedWithdockerFields(&m, msg, "tst-mesos-slave-001")
	js, _ := json.Marshal(msg)
	log.Printf("to json: %s", js)

	assert.Equal("f00ffd9428dc", msg.Fields.Docker.CID)
	assert.Equal("my.registry.host:443/path/to/image", msg.Fields.Docker.Image)

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
