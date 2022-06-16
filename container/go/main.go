package main

import (
	"noc-k8slabels-v1/container/go/pkg/config"
	"noc-k8slabels-v1/container/go/pkg/controller"

	_ "go.uber.org/automaxprocs"

	"github.com/sirupsen/logrus"
)

var c = config.Load()

func main() {
	Run(c)
}

// Run runs the event loop processing with given handler
func Run(conf *config.Config) {

	var eventHandler = ParseEventHandler(conf)
	controller.Start(conf, eventHandler)

}

// ParseEventHandler returns the respective handler object specified in the config file.
func ParseEventHandler(conf *config.Config) controller.Handler {

	var eventHandler controller.Handler = new(Default)

	if err := eventHandler.Init(conf); err != nil {
		logrus.WithField("pkg", "main").Fatal(err)
	}
	return eventHandler
}

// Default handler implements Handler interface,
// print each event with JSON format
type Default struct {
}

// Init initializes handler configuration
// Do nothing for default handler
func (d *Default) Init(c *config.Config) error {
	return nil
}

// ObjectCreated sends events on object creation
func (d *Default) ObjectCreated(obj interface{}) {

}

// ObjectDeleted sends events on object deletion
func (d *Default) ObjectDeleted(obj interface{}) {

}

// ObjectUpdated sends events on object updation
func (d *Default) ObjectUpdated(oldObj, newObj interface{}) {

}

// TestHandler tests the handler configurarion by sending test messages.
func (d *Default) TestHandler() {

}
