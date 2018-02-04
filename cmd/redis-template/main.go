package main

import (
	"flag"
	"log"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/robbert229/envtemplate-redis/pkg"
	"github.com/sirupsen/logrus"
)

type arrayFlags []string

func (i *arrayFlags) String() string {
	return "my string representation"
}

func (i *arrayFlags) Set(value string) error {
	*i = append(*i, value)
	return nil
}

var templateFlags arrayFlags
var redisAddr string
var splay time.Duration
var logger = logrus.New()

func main() {
	flag.Var(&templateFlags, "template", "a template to process")
	flag.StringVar(&redisAddr, "redis-addr", "", "the redis connection string")
	flag.DurationVar(&splay, "splay", time.Duration(0), "This is a random splay to wait before killing the command")

	flag.Parse()

	if redisAddr == "" || len(templateFlags) == 0 {
		flag.Usage()
		return
	}

	pool := redis.Pool{
		Dial: func() (redis.Conn, error) {
			return redis.Dial("tcp", redisAddr)
		},
	}

	var templates []pkg.Template
	for _, templateFlag := range templateFlags {
		templateFlag, err := pkg.ParseTemplateFlag(templateFlag)
		if err != nil {
			log.Fatal("failed to parse template: ", err)
		}

		template, err := templateFlag.ToTemplate(pool)
		if err != nil {
			log.Fatal("failed to load templates: ", err)
		}

		templates = append(templates, template)
		logger.Info("added logger for: ", templateFlag.Source)
	}

	pkg.Poll(logger, pool, templates, splay)
}
