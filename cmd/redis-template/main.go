package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/robbert229/envtemplate-redis/pkg"
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

	cfg := pkg.Config{
		Pool: &redis.Pool{
			Dial: func() (redis.Conn, error) {
				return redis.Dial("tcp", redisAddr)
			},
		},
		Logger:        logger,
		Channel:       pkg.RedisTemplateChannel,
		Splay:         splay,
		TemplateFlags: templateFlags,
	}

	if err := pkg.Listen(cfg); err != nil {
		cfg.Logger.WithError(err).Fatal("failed to listen to redis")
	}
}
