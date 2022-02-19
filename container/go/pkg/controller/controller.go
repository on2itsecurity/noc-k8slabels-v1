/*
Copyright 2016 Skippbox, Ltd.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"noc-k8slabels-v1/container/go/pkg/config"
	"noc-k8slabels-v1/container/go/pkg/panosapi"
	"noc-k8slabels-v1/container/go/pkg/utils"

	"github.com/sirupsen/logrus"

	"github.com/zekroTJA/timedmap"
	api_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/util/workqueue"
)

const maxRetries = 5

// Do not update pod ip/labels if < doNotUpdateFor a create or update was send to the pan api.
const doNotUpdateFor time.Duration = 60 * time.Second

var keyIPmap = timedmap.New(5 * time.Second)
var keyIPmapInterval time.Duration = time.Duration(config.Load().PanFW.RegisterExpire) * time.Second

// Event indicate the informerEvent
type Event struct {
	key       string
	eventType string
	IP        string
	labels    string
}

// Handler is implemented by any handler.
// The Handle method is used to process event
type Handler interface {
	Init(c *config.Config) error
	ObjectCreated(obj interface{})
	ObjectDeleted(obj interface{})
	ObjectUpdated(oldObj, newObj interface{})
	TestHandler()
}

// Controller object
type Controller struct {
	logger       *logrus.Entry
	clientset    kubernetes.Interface
	queue        workqueue.RateLimitingInterface
	informer     cache.SharedIndexInformer
	eventHandler Handler
}

// Start prepares watchers and run their controllers, then waits for process termination signals
func Start(conf *config.Config, eventHandler Handler) {
	var kubeClient kubernetes.Interface

	_, err := rest.InClusterConfig()
	if err != nil {
		logrus.WithField("pkg", "k8slabel-pod").Fatalln("only in cluster config is supported")
	} else {
		kubeClient = utils.GetClient()
	}

	podname := os.Getenv("HOSTNAME")

	kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{})
	namespace, _, err := kubeconfig.Namespace()
	if err != nil {
		namespace = "default"
	}
	// use a Go context so we can tell the leaderelection code when we want to step down
	ctx, cancel := context.WithCancel(context.Background())

	defer cancel()

	stopCh := make(chan struct{})
	defer close(stopCh)
	sigterm := make(chan os.Signal, 1)
	signal.Notify(sigterm, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigterm
		logrus.WithField("pkg", "k8slabel-pod").Info("Received termination, signaling shutdown")
		cancel()
	}()
	// we use the Lease lock type since edits to Leases are less common
	//	 and fewer objects in the cluster watch "all Leases".
	lock := &resourcelock.LeaseLock{
		LeaseMeta: meta_v1.ObjectMeta{
			Name:      "k8slabel",
			Namespace: namespace,
		},
		Client: kubeClient.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: podname,
		},
	}
	/// start the leader election code loop
	leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
		Lock:            lock,
		ReleaseOnCancel: true,
		LeaseDuration:   30 * time.Second,
		RenewDeadline:   10 * time.Second,
		RetryPeriod:     5 * time.Second,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				logrus.WithField("pkg", "k8slabel-pod").Info("Starting k8slabel controller now")

				informer := cache.NewSharedIndexInformer(
					&cache.ListWatch{
						ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
							return kubeClient.CoreV1().Pods("").List(context.Background(), options)
						},
						WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
							return kubeClient.CoreV1().Pods("").Watch(context.Background(), options)
						},
					},
					&api_v1.Pod{},
					time.Duration(conf.Sync.FullResync)*time.Second,
					cache.Indexers{},
				)
				c := newResourceController(kubeClient, eventHandler, informer, "pod")
				go c.Run(stopCh)
			},
			OnStoppedLeading: func() {
				// we can do cleanup here
				logrus.WithField("pkg", "k8slabel-pod").Infof("leader lost: %s", podname)
				os.Exit(0)
			},
			OnNewLeader: func(identity string) {
				// we're notified when new leader elected
				if identity == podname {
					// I just got the lock
					return
				}
				logrus.WithField("pkg", "k8slabel-pod").Infof("new leader elected: %s", identity)
			},
		},
	})

}

func newResourceController(client kubernetes.Interface, eventHandler Handler, informer cache.SharedIndexInformer, resourceType string) *Controller {
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	var newEvent Event
	var err error
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			object := obj.(*api_v1.Pod)
			if err != nil {
				return
			}
			newEvent.key, err = cache.MetaNamespaceKeyFunc(obj)

			newEvent.IP = object.Status.PodIP
			newEvent.labels = createKeyValuePairs(object.GetObjectMeta().GetLabels())

			newEvent.eventType = "create"
			if err == nil && newEvent.labels != "" {
				queue.Add(newEvent)
			}
		},
		UpdateFunc: func(old, new interface{}) {
			object := old.(*api_v1.Pod)
			if err != nil {
				return
			}

			newEvent.key, err = cache.MetaNamespaceKeyFunc(old)
			newEvent.IP = object.Status.PodIP
			newEvent.labels = createKeyValuePairs(object.GetObjectMeta().GetLabels())

			newEvent.eventType = "update"

			if err == nil && newEvent.labels != "" {
				queue.Add(newEvent)
			}
		},
		DeleteFunc: func(obj interface{}) {
			object := obj.(*api_v1.Pod)
			if err != nil {
				return
			}
			if object.Status.PodIP == object.Status.HostIP {
				return
			}

			newEvent.key, err = cache.MetaNamespaceKeyFunc(obj)
			newEvent.IP = object.Status.PodIP
			newEvent.labels = createKeyValuePairs(object.GetObjectMeta().GetLabels())

			newEvent.eventType = "delete"

			if err == nil && newEvent.labels != "" {
				queue.Add(newEvent)
			}
		},
	})

	return &Controller{
		logger:       logrus.WithField("pkg", "k8slabel-"+resourceType),
		clientset:    client,
		informer:     informer,
		queue:        queue,
		eventHandler: eventHandler,
	}
}

// Run starts the noc-k8slabel controller
func (c *Controller) Run(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()
	defer panosapi.Shutdown()

	c.logger.Info("Starting noc-k8slabel controller")

	go c.informer.Run(stopCh)

	if !cache.WaitForCacheSync(stopCh, c.HasSynced) {
		utilruntime.HandleError(fmt.Errorf("timed out waiting for caches to sync"))
		return
	}

	c.logger.Info("noc-k8slabel controller synced and ready")

	wait.Until(c.runWorker, time.Second, stopCh)
}

// HasSynced is required for the cache.Controller interface.
func (c *Controller) HasSynced() bool {
	return c.informer.HasSynced()
}

// LastSyncResourceVersion is required for the cache.Controller interface.
func (c *Controller) LastSyncResourceVersion() string {
	return c.informer.LastSyncResourceVersion()
}

func (c *Controller) runWorker() {
	for c.processNextItem() {
		// continue looping
	}
}

func (c *Controller) processNextItem() bool {
	newEvent, quit := c.queue.Get()

	if quit {
		return false
	}
	defer c.queue.Done(newEvent)
	err := c.processItem(newEvent.(Event))
	if err == nil {
		// No error, reset the ratelimit counters
		c.queue.Forget(newEvent)
	} else if c.queue.NumRequeues(newEvent) < maxRetries {
		c.logger.Debugf("Error processing %s (will retry): %v", newEvent.(Event).key, err)
		time.Sleep(time.Duration(c.queue.NumRequeues(newEvent)) * (time.Second / 10))
		c.queue.AddRateLimited(newEvent)
	} else {
		// err != nil and too many retries
		c.logger.Debugf("Error processing %s (giving up): %v", newEvent.(Event).key, err)
		c.queue.Forget(newEvent)
		//utilruntime.HandleError(err)
	}

	return true
}

/* TODOs
- Enhance event creation using client-side cacheing machanisms - pending
- Enhance the processItem to classify events - done
- Send alerts correspoding to events - done
*/

func (c *Controller) processItem(newEvent Event) error {
	// process events based on its type

	switch newEvent.eventType {
	case "create":
		obj, _, err := c.informer.GetIndexer().GetByKey(newEvent.key)
		if err != nil {
			return fmt.Errorf("error fetching object with key %s from store: %v", newEvent.key, err)
		}
		if obj == nil {
			return fmt.Errorf("event with key %s has no object", newEvent.key)
		}
		object := obj.(*api_v1.Pod)

		if net.ParseIP(object.Status.PodIP) == nil {
			return fmt.Errorf("pod %s has no ip, ignoring for now", object.GetObjectMeta().GetName())
		}
		if object.Status.PodIP == object.Status.HostIP {
			c.logger.Debugf("Ignoring pod on HostIP %s", object.GetObjectMeta().GetName())
			return nil
		}

		newEvent.IP = object.Status.PodIP
		keyIPmap.Set(newEvent.key, newEvent.IP, keyIPmapInterval)

		c.logger.Infof("Processing create %s with labels %s", newEvent.IP, newEvent.labels)
		panosapi.BatchUpdateIPs(net.ParseIP(newEvent.IP), newEvent.labels)

		return nil
	case "update":
		obj, _, err := c.informer.GetIndexer().GetByKey(newEvent.key)
		if err != nil {
			return fmt.Errorf("error fetching object with key %s from store: %v", newEvent.key, err)
		}
		if obj == nil {
			return nil
		}
		object := obj.(*api_v1.Pod)

		if net.ParseIP(newEvent.IP) == nil {
			newEvent.IP = object.Status.PodIP
		}

		if object.Status.HostIP != "" && newEvent.IP == object.Status.HostIP {
			c.logger.Debugf("Ignoring pod on HostIP %s", object.GetObjectMeta().GetName())
			return nil
		}
		switch object.Status.Phase {
		case "Pending", "Running":
			if net.ParseIP(newEvent.IP) == nil {
				return fmt.Errorf("pod %s has no ip, ignoring for now", object.GetObjectMeta().GetName())
			}
			// if pod is Terminating, we want to retain the labels, let the delete event remove the ip label entries
			if object.DeletionTimestamp != nil {
				return nil
			}
			keyExpire, err := keyIPmap.GetExpires(newEvent.key)
			if keyExpire.After(time.Now().Add(keyIPmapInterval-doNotUpdateFor)) && err == nil {
				return nil
			}
			c.logger.Infof("Processing update %s with labels %s", newEvent.IP, newEvent.labels)
			panosapi.BatchUpdateIPs(net.ParseIP(newEvent.IP), newEvent.labels)
			keyIPmap.Set(newEvent.key, newEvent.IP, keyIPmapInterval)
		case "Unknown", "Failed", "Succeeded":
			if net.ParseIP(newEvent.IP) == nil {
				if keyIPmap.GetValue(newEvent.key).(string) == "" {
					return nil
				}
				newEvent.IP = keyIPmap.GetValue(newEvent.key).(string)
				if net.ParseIP(newEvent.IP) == nil {
					return nil
				}
			}
			c.logger.Infof("Removing %s with labels %s", newEvent.IP, newEvent.labels)
			panosapi.BatchRemoveIPs(net.ParseIP(newEvent.IP), newEvent.labels)
			keyIPmap.Remove(newEvent.key)
		}
		return nil
	case "delete":
		if net.ParseIP(newEvent.IP) == nil {
			newEvent.IP = keyIPmap.GetValue(newEvent.key).(string)
			if net.ParseIP(newEvent.IP) == nil {
				return nil
			}
		}
		if keyIPmap.GetValue(newEvent.key).(string) == "" {
			return nil
		}
		c.logger.Infof("Removing %s with labels %s", newEvent.IP, newEvent.labels)
		panosapi.BatchRemoveIPs(net.ParseIP(newEvent.IP), newEvent.labels)
		keyIPmap.Remove(newEvent.key)
		return nil
	}
	return nil
}
func createKeyValuePairs(m map[string]string) string {
	conf := config.Load()
	b := new(bytes.Buffer)
	for key, value := range m {
		if conf.Sync.LabelKeys == "" || inArray(key, strings.Split(conf.Sync.LabelKeys, ",")) {
			fmt.Fprintf(b, "%s=%s,", key, value)
		}
	}
	return strings.TrimSuffix(b.String(), ",")
}
func inArray(val string, array []string) (exists bool) {
	exists = false

	for _, v := range array {
		if val == v {
			exists = true
			return
		}
	}

	return
}
