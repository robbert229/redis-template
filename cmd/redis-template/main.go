package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/robbert229/redis-template/pkg"
	"github.com/sirupsen/logrus"
)

var templateFlags pkg.TemplateFlags
var redisAddr string
var splay time.Duration
var redisChannel string
var logLevel string

const (
	LogLevelDebug = "DEBUG"
	LogLevelInfo  = "INFO"
	LogLevelWarn  = "WARN"
	LogLevelError = "ERROR"
)

func main() {
	flag.Var(&templateFlags, "template", "a template to process")
	flag.StringVar(&redisAddr, "redis-addr", "", "the redis connection string")
	flag.StringVar(&redisChannel, "redis-chan", pkg.RedisTemplateChannel, "the redis channel to listen for updates on")
	flag.DurationVar(&splay, "splay", time.Duration(0), "This is a random splay to wait before killing the command")
	flag.StringVar(&logLevel, "log-level", LogLevelError, fmt.Sprintf("the logging level. (%s|%s|%s|%s)",
		LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError))

	flag.Parse()

	if redisAddr == "" {
		fmt.Println("no redis address given")
		flag.Usage()
		return
	}

	if len(templateFlags) == 0 {
		fmt.Println("no templates given")
		flag.Usage()
		return
	}

	logger := logrus.New()

	switch logLevel {
	case LogLevelDebug:
		logger.SetLevel(logrus.DebugLevel)
	case LogLevelInfo:
		logger.SetLevel(logrus.InfoLevel)
	case LogLevelWarn:
		logger.SetLevel(logrus.WarnLevel)
	case LogLevelError:
		logger.SetLevel(logrus.ErrorLevel)
	default:
		fmt.Println("invalid log-level given: ", logLevel)
		flag.Usage()
		return
	}

	pool := &redis.Pool{
		Dial: func() (redis.Conn, error) {
			return redis.Dial("tcp", redisAddr)
		},
	}

	// parse all of the templates and anchor the redis pool into scope.
	templates := make([]pkg.Template, len(templateFlags))
	for i := 0; i < len(templateFlags); i++ {
		tmpl, err := templateFlags[i].ToTemplate(pool)
		if err != nil {
			logger.WithError(err).Fatalf("failed build template")
		}

		templates[i] = tmpl
	}

	cfg := pkg.Config{
		Pool:      pool,
		Logger:    logger,
		Channel:   pkg.RedisTemplateChannel,
		Splay:     splay,
		Templates: templates,
	}

	if err := pkg.Listen(cfg); err != nil {
		cfg.Logger.WithError(err).Fatal("failed to listen to redis")
	}
}
