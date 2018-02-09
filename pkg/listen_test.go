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
	Cleanup func() error
	Logger  *logrus.Logger
}

// SetupTestEnvironment creates a new test environment.
func SetupTestEnvironment(port int, t *testing.T) TestEnvironment {
	logger := logrus.New()

	host := "localhost"

	redisDockerContainerName := fmt.Sprintf("redis-template-testing-%d", port)

	// Setup an instance of redis to test against.
	cmd := exec.Command("docker", "run", "--name", redisDockerContainerName, "-d", "-p", fmt.Sprintf("%d:6379", port), "--rm", "redis")
	if bytes, err := cmd.CombinedOutput(); err != nil {
		logger.Info(string(bytes))
		t.Fatal("failed to start docker container: ", err)
	}

	// create a Cleanup helper function.
	cleanupDone := false
	cleanup := func() error {
		if cleanupDone {
			return nil
		}

		cleanupDone = true
		cmd := exec.Command("docker", "rm", "-f", redisDockerContainerName)
		if bytes, err := cmd.CombinedOutput(); err != nil {
			logger.Info(string(bytes))
			return errors.Wrap(err, "failed to stop docker container")
		}

		return nil
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

		conn.Close()
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

	if _, err := conn.Do("SET", "count", "1"); err != nil {
		t.Fatal(err)
	}

	actionCount := 0

	wg := sync.WaitGroup{}
	wg.Add(1)

	go func() {
		err := Listen(Config{
			Logger:  env.Logger,
			Channel: RedisTemplateChannel,
			Splay:   time.Duration(0),
			Templates: []Template{
				MustTemplate(t, env.Pool, TemplateFlag{
					Source: TestTemplate,
					Target: TestOutput,
				}, func() error {
					actionCount++
					return nil
				}),
			},
			Pool: env.Pool,
		})

		fmt.Println(err)

		wg.Done()
	}()

	// wait until the actionCount is incremented.
	for actionCount != 1 {
		time.Sleep(time.Second)
	}

	if _, err := conn.Do("SET", "count", "2"); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Do("PUBLISH", RedisTemplateChannel, "."); err != nil {
		t.Fatal(err)
	}

	// wait until the actionCount is incremented again.
	for {
		if actionCount != 2 {
			break
		}

		time.Sleep(time.Second)
	}

	for i := 0; i < 100; i++ {
		// lets now test that when the pubsub on redis updates a bunch of times but the computed templates
		if _, err := conn.Do("PUBLISH", RedisTemplateChannel, "."); err != nil {
			t.Fatal(err)
		}
	}

	time.Sleep(time.Second)
	if actionCount != 2 {
		t.Fatalf("actionCount is not updated. expected: 2, actual: %d", actionCount)
	}

	if err := env.Cleanup(); err != nil {
		t.Fatal(err)
	}

	wg.Wait()
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

	if _, err := conn.Do("SET", "foo", "Hello!!"); err != nil {
		t.Fatal(err)
	}

	template, err := TemplateFlag{
		Source: TestTemplate,
		Target: TestOutput,
	}.ToTemplate(env.Pool)

	actionCount := 0
	template.Action = func() error {
		actionCount++
		return nil
	}

	wg := sync.WaitGroup{}
	wg.Add(1)

	go func() {
		err := Listen(Config{
			Logger:    env.Logger,
			Channel:   RedisTemplateChannel,
			Splay:     time.Duration(0),
			Templates: []Template{template},
			Pool:      env.Pool,
		})

		fmt.Println(err)

		wg.Done()
	}()

	// wait until the actionCount is incremented.
	for {
		if actionCount != 0 {
			break
		}

		time.Sleep(time.Second)
	}

	if _, err := conn.Do("SET", "foo", "Hello"); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Do("PUBLISH", RedisTemplateChannel, "."); err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second)

	actualBytes, err := ioutil.ReadFile(TestOutput)
	if err != nil {
		t.Fatal(err)
	}

	expected := []string{"Hello", "World"}
	assert.Equal(t, strings.Split(string(actualBytes), "\n"), expected)

	if err := env.Cleanup(); err != nil {
		t.Fatal(err)
	}

	wg.Wait()
}
