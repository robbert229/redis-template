package pkg

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"sync"

	"github.com/garyburd/redigo/redis"
	"github.com/magiconair/properties/assert"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const RedisDockerContainerName = "redis-template-test-redis"
const HostPort = 6378

const TestTemplate = "./test.json.tpl"
const TestOutput = "./test.json"
const TestLog = "./test.log"

func TestListen(t *testing.T) {
	os.Remove(TestTemplate)
	os.Remove(TestOutput)
	os.Remove(TestLog)

	// Setup an instance of redis to test against.
	cmd := exec.Command("docker", "run", "--name", RedisDockerContainerName, "-d", "-p", fmt.Sprintf("%d:6379", HostPort), "--rm", "redis")
	if bytes, err := cmd.CombinedOutput(); err != nil {
		t.Log(string(bytes))
		t.Fatal("failed to start docker container: ", err)
	}

	// create a cleanup helper function.
	cleanupDone := false
	cleanup := func() {
		if cleanupDone {
			return
		}

		cleanupDone = true
		cmd := exec.Command("docker", "rm", "-f", RedisDockerContainerName)
		if bytes, err := cmd.CombinedOutput(); err != nil {
			t.Log(string(bytes))
			t.Fatal("failed to stop docker container: ", err)
		}

	}

	// execute the cleanup function at the end of the function.
	defer cleanup()

	// create the configuration
	pool := &redis.Pool{
		Dial: func() (redis.Conn, error) {
			conn, err := redis.Dial("tcp", fmt.Sprintf("localhost:%d", HostPort))
			return conn, errors.WithStack(err)
		},
	}

	logger := logrus.New()
	logger.Out = ioutil.Discard

	testTemplate := `{{key "foo"}}
{{keyOrDefault "bar" "World"}}`
	err := ioutil.WriteFile(TestTemplate, []byte(testTemplate), 0755)
	if err != nil {
		t.Fatal(err)
	}

	conn, err := pool.Dial()
	if err != nil {
		t.Fatal(err)
	}

	if _, err := conn.Do("SET", "foo", "Hello!!"); err != nil {
		t.Fatal(err)
	}

	wg := sync.WaitGroup{}
	wg.Add(1)

	go func() {
		err := Listen(Config{
			Logger:  logger,
			Channel: RedisTemplateChannel,
			Splay:   time.Duration(0),
			TemplateFlags: []TemplateFlag{
				{
					Source: TestTemplate,
					Target: TestOutput,
					Action: fmt.Sprintf(`echo "value" > %s`, TestLog),
				},
			},
			Pool: pool,
		})

		fmt.Println(err)

		wg.Done()
	}()

	// wait until the log file exists
	for {
		_, err := os.Stat(TestLog)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}

			t.Fatal(err)
		} else {
			break
		}
	}

	// now that we know that redis-template has executed lets test that we can get it going on again.
	if err := os.Remove(TestLog); err != nil {
		t.Fatal(err)
	}

	if _, err := conn.Do("SET", "foo", "Hello"); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Do("PUBLISH", RedisTemplateChannel, "."); err != nil {
		t.Fatal(err)
	}

	// wait until the log file exists
	for {
		_, err := os.Stat(TestLog)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}

			t.Fatal(err)
		} else {
			break
		}
	}

	cleanup()
	wg.Wait()

	actualBytes, err := ioutil.ReadFile(TestOutput)
	if err != nil {
		t.Fatal(err)
	}

	expected := []string{"Hello", "World"}
	assert.Equal(t, strings.Split(string(actualBytes), "\n"), expected)

	os.Remove(TestLog)
	os.Remove(TestOutput)
	os.Remove(TestTemplate)
}
