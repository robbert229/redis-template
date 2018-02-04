package pkg

import (
	"bytes"
	"io/ioutil"
	"math/rand"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/garyburd/redigo/redis"
)

const RedisTemplateChannel = "redis-template-channel"

func Poll(log *log.Logger, pool redis.Pool, templates []Template, splay time.Duration) error {
	previousTemplateExecutions := map[string]string{}

	c, err := pool.Dial()
	if err != nil {
		return err
	}

	psc := redis.PubSubConn{c}
	if err := psc.Subscribe(RedisTemplateChannel); err != nil {
		return err
	}

	for _, template := range templates {
		buffer := bytes.NewBuffer(nil)
		if err := template.SourceTemplate.Execute(buffer, nil); err != nil {
			log.Fatal("error: failed to execute template: ", err)
		}

		if previousTemplateExecutions[template.SourceTemplate.Name()] != buffer.String() {
			if err := ioutil.WriteFile(template.Target, buffer.Bytes(), 0666); err != nil {
				log.Fatal("error: failed to write template to file: ", err)
			}
		}

		previousTemplateExecutions[template.SourceTemplate.Name()] = buffer.String()
	}

	log.Info("subscribed to: ", RedisTemplateChannel)

	for {
		reply := psc.Receive()
		log.WithField("reply", reply).Info("message received")

		switch v := reply.(type) {
		case redis.Message:
			log.Info("update detected, reloading templates")

			splayMs := int64(splay / time.Millisecond)
			randomWait := time.Duration(int64(rand.Float64() * float64(splayMs)))
			time.Sleep(randomWait)

			for _, template := range templates {
				buffer := bytes.NewBuffer(nil)
				if err := template.SourceTemplate.Execute(buffer, nil); err != nil {
					log.Error("error: failed to execute template: ", err)
					continue
				}

				if previousTemplateExecutions[template.SourceTemplate.Name()] != buffer.String() {
					if err := ioutil.WriteFile(template.Target, buffer.Bytes(), 0666); err != nil {
						log.Error("error: failed to write template to file: ", err)
						continue
					}
				}

				if err := template.Execute(); err != nil {
					log.Error("error: failed to execute command: ", err)
					continue
				}

				previousTemplateExecutions[template.SourceTemplate.Name()] = buffer.String()
			}
		case error:
			return v
		}
	}
}
