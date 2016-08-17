package main

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/facebookgo/httpdown"

	"github.com/mozilla-services/go-syncstorage/config"
	"github.com/mozilla-services/go-syncstorage/web"
)

func init() {
	switch config.Log.Level {
	case "fatal":
		log.SetLevel(log.FatalLevel)
	case "error":
		log.SetLevel(log.ErrorLevel)
	case "warn":
		log.SetLevel(log.WarnLevel)
	case "debug":
		log.SetLevel(log.DebugLevel)
	default:
		log.SetLevel(log.InfoLevel)
	}
}

func main() {

	var router http.Handler

	syncLimitConfig := web.NewDefaultSyncUserHandlerConfig()
	if config.Limit.MaxRequestBytes != 0 {
		syncLimitConfig.MaxRequestBytes = config.Limit.MaxRequestBytes
	}
	if config.Limit.MaxBSOGetLimit != 0 {
		syncLimitConfig.MaxBSOGetLimit = config.Limit.MaxBSOGetLimit
	}
	if config.Limit.MaxPOSTRecords != 0 {
		syncLimitConfig.MaxPOSTRecords = config.Limit.MaxPOSTRecords
	}
	if config.Limit.MaxPOSTBytes != 0 {
		syncLimitConfig.MaxPOSTBytes = config.Limit.MaxPOSTBytes
	}
	if config.Limit.MaxTotalBytes != 0 {
		syncLimitConfig.MaxTotalBytes = config.Limit.MaxTotalBytes
	}
	if config.Limit.MaxTotalRecords != 0 {
		syncLimitConfig.MaxTotalRecords = config.Limit.MaxTotalRecords
	}
	if config.Limit.MaxBatchTTL != 0 {
		syncLimitConfig.MaxBatchTTL = config.Limit.MaxBatchTTL * 1000
	}

	// The base functionality is the sync 1.5 api + legacy weave hacks
	poolHandler := web.NewSyncPoolHandler(&web.SyncPoolConfig{
		Basepath:    config.DataDir,
		NumPools:    config.Pool.Num,
		MaxPoolSize: config.Pool.MaxSize,
	}, syncLimitConfig)
	router = web.NewWeaveHandler(poolHandler)

	// All sync 1.5 access requires Hawk Authorization
	router = web.NewHawkHandler(router, config.Secrets)

	// Serve non sync 1.5 endpoints
	router = web.NewInfoHandler(router)

	// Log all the things
	if config.Log.DisableHTTP != true {
		router = web.NewLogHandler(router)
	}

	if config.EnablePprof {
		log.Info("Enabling pprof profile at /debug/pprof/")
		router = web.NewPprofHandler(router)
	}

	listenOn := config.Host + ":" + strconv.Itoa(config.Port)
	server := &http.Server{
		Addr:    listenOn,
		Handler: router,
	}

	if config.Log.Mozlog {
		log.SetFormatter(&web.MozlogFormatter{
			Hostname: config.Hostname,
			Pid:      os.Getpid(),
		})
	}

	hd := &httpdown.HTTP{
		// how long until connections are force closed
		StopTimeout: 3 * time.Minute,

		// how long before complete abort (even when clients are connected)
		// this is above StopTimeout. In other worse, how much time to give
		// force stopping of connections to finish
		KillTimeout: 2 * time.Minute,
	}

	log.WithFields(log.Fields{
		"addr":                    listenOn,
		"PID":                     os.Getpid(),
		"POOL_NUM":                config.Pool.Num,
		"POOL_MAX_SIZE":           config.Pool.MaxSize,
		"LIMIT_MAX_BSO_GET_LIMIT": syncLimitConfig.MaxBSOGetLimit,
		"LIMIT_MAX_POST_RECORDS":  syncLimitConfig.MaxPOSTRecords,
		"LIMIT_MAX_POST_BYTES":    syncLimitConfig.MaxPOSTBytes,
		"LIMIT_MAX_TOTAL_RECORDS": syncLimitConfig.MaxTotalRecords,
		"LIMIT_MAX_TOTAL_BYTES":   syncLimitConfig.MaxTotalBytes,
		"LIMIT_MAX_REQUEST_BYTES": syncLimitConfig.MaxRequestBytes,
		"LIMIT_MAX_BATCH_TTL":     fmt.Sprintf("%d seconds", syncLimitConfig.MaxBatchTTL/1000),
	}).Info("HTTP Listening at " + listenOn)

	err := httpdown.ListenAndServe(server, hd)
	if err != nil {
		log.Error(err.Error())
	}

	poolHandler.StopHTTP()
}
