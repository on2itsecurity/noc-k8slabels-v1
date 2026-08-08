package config

import (
	"flag"

	"github.com/sirupsen/logrus"
	"gopkg.in/ini.v1"
)

var (
	iniFileName = flag.String("inifile", "env.ini", "which contains the pan xml-api credentials")
	verbose     = flag.Bool("verbose", false, "See api-calls")
)
var (
	iniFile *ini.File
)

// Config struct contains panfw-dag-k8slabels configuration
type Config struct {
	PanFW *PanFW
	Sync  *Sync
}

// Sync struct contains k8s sync configuration - part of Config struct
type Sync struct {
	// for watching specific namespace, leave it empty for watching all.
	// this config is ignored when watching namespaces
	Namespace       string
	LabelKeys       string
	FullResync      int
	PrefixLabelKeys string
}

// PanFW struct contains Palo Alto Networks configuration - part of Config struct
type PanFW struct {
	Token          string
	Username       string
	Password       string
	URL            []string `ini:",,allowshadow"`
	RegisterExpire int
}

// Load loads configuration from config file
func Load() *Config {
	// Default values
	sync := &Sync{
		Namespace:       "",
		FullResync:      15 * 60,
		LabelKeys:       "",
		PrefixLabelKeys: "",
	}
	panfw := &PanFW{
		Token:          "",
		Username:       "",
		Password:       "",
		URL:            []string{},
		RegisterExpire: 60 * 60,
	}

	// Parse command runtime parameters
	flag.Parse()

	if *verbose {
		logrus.SetLevel(logrus.DebugLevel)
	}
	// Load ini file
	var err error
	iniFile, err = ini.ShadowLoad(*iniFileName)
	if err != nil {
		logrus.WithField("pkg", "config").Fatalln("Fail to read inifile", err)
	}

	// Parse SYNC section
	err = iniFile.Section("SYNC").MapTo(sync)
	if err != nil {
		logrus.WithField("pkg", "config").Fatalln("Fail to parse SYNC section", err)
	}

	// Parse PAN-FW section
	err = iniFile.Section("PAN-FW").MapTo(panfw)
	if err != nil {
		logrus.WithField("pkg", "config").Fatalln("Fail to parse PAN-FW section", err)
	}
	c := &Config{
		Sync:  sync,
		PanFW: panfw,
	}
	return c
}
