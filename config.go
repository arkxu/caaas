package main

import (
	"fmt"

	"github.com/golang/glog"
	"github.com/spf13/viper"
)

type HostConfig struct {
	Host string
	Port string
}

type Auth struct {
	Username string
	Password string
}

type DbConfig struct {
	Hosts    []string
	DBName   string
	NumConns int
	Timeout  int
}

type ImageConfig struct {
	StoreWidth     int
	StoreHeight    int
	DefaultWidth   int
	DefaultHeight  int
	StoreQuality   int
	ReadQuality    int
	CacheDir       string
	CacheDirLength int
	UseGoRoutine   bool
	ProcessPar     int
}

type Configuration struct {
	Http  *HostConfig
	Auth  *Auth
	Db    *DbConfig
	Image *ImageConfig
}

var (
	Config       *Configuration
	ImageChannel chan int
)

func init() {
	viper.SetConfigName("caaas")        // name of config file (without extension)
	viper.AddConfigPath("/etc/caaas/")  // path to look for the config file in
	viper.AddConfigPath("$HOME/.caaas") // call multiple times to add many search paths
	viper.AddConfigPath(".")            // optionally look for config in the working directory
	err := viper.ReadInConfig()         // Find and read the config file
	if err != nil {                     // Handle errors reading the config file
		glog.Fatal("Can not read config file", err)
		panic(fmt.Errorf("Fatal error config file: %s \n", err))
	}

	httpConfig := &HostConfig{viper.GetString("http.host"), viper.GetString("http.port")}
	authConfig := &Auth{viper.GetString("auth.username"), viper.GetString("auth.password")}
	imageConfig := &ImageConfig{
		StoreWidth:     viper.GetInt("image.storeWidth"),
		StoreHeight:    viper.GetInt("image.storeHeight"),
		DefaultWidth:   viper.GetInt("image.defaultWidth"),
		DefaultHeight:  viper.GetInt("image.defaultHeight"),
		StoreQuality:   viper.GetInt("image.storeQuality"),
		ReadQuality:    viper.GetInt("image.readQuality"),
		CacheDir:       viper.GetString("image.cacheDir"),
		CacheDirLength: viper.GetInt("image.cacheDirLength"),
		UseGoRoutine:   viper.GetBool("image.useGoRoutine"),
		ProcessPar:     viper.GetInt("image.processPar"),
	}
	dbConfig := &DbConfig{viper.GetStringSlice("db.hosts"),
		viper.GetString("db.dbName"), viper.GetInt("db.numConns"), viper.GetInt("db.timeout")}

	Config = &Configuration{
		Http:  httpConfig,
		Auth:  authConfig,
		Db:    dbConfig,
		Image: imageConfig,
	}

	// semaphore for max number of image processor run in parallel
	ImageChannel = make(chan int, Config.Image.ProcessPar)
}
