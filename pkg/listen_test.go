package pkg

import (
	"fmt"
	"io/ioutil"
	"os/exec"
	"strings"
	"testing"
	"time"

	"sync"

	"github.com/garyburd/redigo/redis"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

// TestEnvironment control the test environment.
type TestEnvironment struct {
	Pool    *redis.Pool
	Cleanup func()
	Logger  *logrus.Logger
}

// SetupTestEnvironment creates a new test environment.
func SetupTestEnvironment(port int, t *testing.T) TestEnvironment {
	logger := logrus.New()

	host := "localhost"

	redisDockerContainerName := fmt.Sprintf("redis-template-testing-%d", port)

	cmd := exec.Command("docker", "rm", "-f", redisDockerContainerName)
	if bytes, err := cmd.CombinedOutput(); err != nil {
		logger.Warn(string(bytes))
		logger.WithError(err).Warn("failed to delete docker container: ", err)
	}

	// Setup an instance of redis to test against.
	cmd = exec.Command("docker", "run", "--name", redisDockerContainerName, "-d", "-p", fmt.Sprintf("%d:6379", port), "--rm", "redis")
	if bytes, err := cmd.CombinedOutput(); err != nil {
		logger.Warn(string(bytes))
		t.Fatal("failed to start docker container: ", err)
	}

	// create a Cleanup helper function.
	cleanupDone := false
	cleanup := func() {
		if cleanupDone {
			return
		}

		cleanupDone = true
		cmd := exec.Command("docker", "rm", "-f", redisDockerContainerName)
		if bytes, err := cmd.CombinedOutput(); err != nil {
			logger.Warn(string(bytes))
			t.Fatal("failed to stop docker container")
		}
	}

	// create the configuration
	pool := &redis.Pool{
		Dial: func() (redis.Conn, error) {
			conn, err := redis.Dial("tcp", fmt.Sprintf("%s:%d", host, port))
			return conn, errors.WithStack(err)
		},
	}

	for {
		conn, err := pool.Dial()
		if err != nil {
			logger.Info(err)
			time.Sleep(time.Second / 10)
			continue
		}

		err = conn.Close()
		if err != nil {
			t.Fatal(err)
		}

		break
	}

	return TestEnvironment{
		Pool:    pool,
		Cleanup: cleanup,
		Logger:  logger,
	}
}

// MustTemplate is a helper that fails the test when a flag cannot be turned into a template
func MustTemplate(t *testing.T, pool *redis.Pool, flag TemplateFlag, action func() error) Template {
	template, err := flag.ToTemplate(pool)
	if err != nil {
		t.Fatal(err)
	}

	template.Action = action
	return template
}

// TestListen_ExecuteAction is designed to thoroughly test the execution of Template Actions.
func TestListen_ExecuteAction(t *testing.T) {
	const TestTemplate = "./test_files/execute.tmpl"
	const TestOutput = "./test_files/execute.out"

	env := SetupTestEnvironment(6377, t)
	defer env.Cleanup()

	err := ioutil.WriteFile(TestTemplate, []byte(`{{key "count"}}`), 0755)
	if err != nil {
		t.Fatal(err)
	}

	conn, err := env.Pool.Dial()
	if err != nil {
		t.Fatal(err)
	}

	_, err = conn.Do("SET", "count", "1")
	if err != nil {
		t.Fatal(err)
	}

	actionCount := 0
	mut := &sync.Mutex{}
	wg := &sync.WaitGroup{}
	wg.Add(1)

	var listenErr error
	go func() {
		listenErr = Listen(Config{
			Logger:  env.Logger,
			Channel: RedisTemplateChannel,
			Splay:   time.Duration(0),
			Templates: []Template{
				MustTemplate(t, env.Pool, TemplateFlag{
					Source: TestTemplate,
					Target: TestOutput,
				}, func() error {
					mut.Lock()
					actionCount++
					mut.Unlock()
					return nil
				}),
			},
			Pool: env.Pool,
		})

		wg.Done()
	}()

	// wait until the actionCount is incremented.
	for {
		mut.Lock()
		cur := actionCount
		mut.Unlock()

		if cur == 1 {
			break
		}

		time.Sleep(time.Second)
	}

	_, err = conn.Do("SET", "count", "2")
	if err != nil {
		t.Fatal(err)
	}

	_, err = conn.Do("PUBLISH", RedisTemplateChannel, ".")
	if err != nil {
		t.Fatal(err)
	}

	// wait until the actionCount is incremented again.
	for {
		mut.Lock()
		cur := actionCount
		mut.Unlock()

		if cur == 2 {
			break
		}

		time.Sleep(time.Second)
	}

	for i := 0; i < 100; i++ {
		// lets now test that when the pubsub on redis updates a bunch of times but the computed templates
		_, err = conn.Do("PUBLISH", RedisTemplateChannel, ".")
		if err != nil {
			t.Fatal(err)
		}
	}

	time.Sleep(time.Second)
	mut.Lock()
	cur := actionCount
	mut.Unlock()
	if cur != 2 {
		t.Fatalf("actionCount is not updated. expected: 2, actual: %d", cur)
	}

	env.Cleanup()
	wg.Wait()

	// ensure that there is an EOF error if one exists.
	if listenErr != nil && listenErr.Error() != "EOF" {
		t.Fatal(err)
	}
}

// TestListen_WritingTemplate tests that templates are properly executed and written.
func TestListen_WritingTemplate(t *testing.T) {
	const TestTemplate = "./test_files/template.json.tmpl"
	const TestOutput = "./test_files/template.json"

	env := SetupTestEnvironment(6378, t)
	defer env.Cleanup()

	testTemplate := `{{key "foo"}}
{{keyOrDefault "bar" "World"}}`
	err := ioutil.WriteFile(TestTemplate, []byte(testTemplate), 0755)
	if err != nil {
		t.Fatal(err)
	}

	conn, err := env.Pool.Dial()
	if err != nil {
		t.Fatal(err)
	}

	_, err = conn.Do("SET", "foo", "Hello!!")
	if err != nil {
		t.Fatal(err)
	}

	template, err := TemplateFlag{
		Source: TestTemplate,
		Target: TestOutput,
	}.ToTemplate(env.Pool)
	if err != nil {
		t.Fatal(err)
	}

	mut := &sync.Mutex{}
	actionCount := 0

	template.Action = func() error {
		mut.Lock()
		actionCount++
		mut.Unlock()
		return nil
	}

	wg := sync.WaitGroup{}
	wg.Add(1)

	var listenErr error
	go func() {
		listenErr = Listen(Config{
			Logger:    env.Logger,
			Channel:   RedisTemplateChannel,
			Splay:     time.Duration(0),
			Templates: []Template{template},
			Pool:      env.Pool,
		})

		wg.Done()
	}()

	// wait until the actionCount is incremented.
	for {
		mut.Lock()
		cur := actionCount
		mut.Unlock()

		if cur != 0 {
			break
		}

		time.Sleep(time.Second)
	}

	_, err = conn.Do("SET", "foo", "Hello")
	if err != nil {
		t.Fatal(err)
	}

	_, err = conn.Do("PUBLISH", RedisTemplateChannel, ".")
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second)

	actualBytes, err := ioutil.ReadFile(TestOutput)
	if err != nil {
		t.Fatal(err)
	}

	expected := []string{"Hello", "World"}
	assert.Equal(t, strings.Split(string(actualBytes), "\n"), expected)

	env.Cleanup()
	wg.Wait()

	// check that there isn't an error or it was the EOF error.
	if listenErr != nil && listenErr.Error() != "EOF" {
		t.Fatal(err)
	}
}
