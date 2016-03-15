package redis

import (
	"testing"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/gliderlabs/logspout/router"
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

	msg, _ := createLogstashMessage(&m, "tst-mesos-slave-001", false, "my-type")
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

	msg, _ := createLogstashMessage(&m, "tst-mesos-slave-001", true, "some-type")
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

	msg, _ := createLogstashMessage(&m, "tst-mesos-slave-001", true, "")
	jq := makeQuery(msg)

	assert.Equal("", getString(jq, "@type"))

}

func TestLooksToBeJsonMessage(t *testing.T) {
	assert := assert.New(t)

	assert.False(looksToBeJsonMessage("ffff"))
	assert.True(looksToBeJsonMessage("{ffff}"))
	assert.False(looksToBeJsonMessage(""))

}

func TestIsGenericJsonMessage(t *testing.T) {
	assert := assert.New(t)

	/*js := `{
	  "@source_host":"test.here.com",
	  "@timestamp":"2013-10-24T09:30:46.947024155+02:00",
	  "@fields":{
	       "docker": {
			  "name": "/my_db",
			  "cid":"dsfbgrgfer45t",
	          "image": "my.registry.host:443/path/to/image",
	          "image_tag": "0.1.1",
	          "source":"stderr",
			  "docker_host":"tst-mesos-slave-001"
	       },
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
	*/
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

	assert.True(isGenericJsonMessage(js))

}

func TestIsOfAllowedType(t *testing.T) {
	assert := assert.New(t)

	js := `{
  "@source_host":"test.here.com",
  "@timestamp":"2013-10-24T09:30:46.947024155+02:00",
  "@fields":{
       "log_type": "generfic",
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

	assert.False(isOfAllowedType(js))

	js = `{
  "@source_host":"test.here.com",
  "@timestamp":"2013-10-24T09:30:46.947024155+02:00",
  "@fields":{
       "log_type": "",
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

	assert.False(isOfAllowedType(js))

	js = `{
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

	assert.True(isOfAllowedType(js))

}
