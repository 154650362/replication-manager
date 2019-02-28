// replication-manager - Replication Manager Monitoring and CLI for MariaDB and MySQL
// Copyright 2017 Signal 18 SARL
// Author: Guillaume Lefranc <guillaume@signal18.io>
// License: GNU General Public License, version 3. Redistribution/Reuse of this code is permitted under the GNU v3 license, as an additional term ALL code must carry the original Author(s) credit in comment form.
// See LICENSE in this directory for the integral text.

package main

import (
	"io/ioutil"
	mysqllog "log"
	"os"
	"runtime/pprof"
	"strconv"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"

	"github.com/go-sql-driver/mysql"
	termbox "github.com/nsf/termbox-go"
	uuid "github.com/satori/go.uuid"
	"github.com/signal18/replication-manager/cluster"
	"github.com/signal18/replication-manager/graphite"
	"github.com/signal18/replication-manager/opensvc"
	"github.com/signal18/replication-manager/termlog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Global variables
var (
	tlog           termlog.TermLog
	termlength     int
	runUUID        string
	repmgrHostname string
	runStatus      string
	splitBrain     bool
	swChan         = make(chan bool)
	exitMsg        string
	exit           bool
	currentCluster *cluster.Cluster
	clusters       = map[string]*cluster.Cluster{}
	agents         []opensvc.Host
)

func init() {
	runUUID = uuid.NewV4().String()
	runStatus = "A"
	splitBrain = false
	conf.FailForceGtid = true
	//	conf := confs[cfgGroup]
	var errLog = mysql.Logger(mysqllog.New(ioutil.Discard, "", 0))
	mysql.SetLogger(errLog)
	rootCmd.AddCommand(switchoverCmd)
	rootCmd.AddCommand(failoverCmd)
	rootCmd.AddCommand(monitorCmd)
	rootCmd.AddCommand(topologyCmd)
	initRepmgrFlags(switchoverCmd)
	initRepmgrFlags(failoverCmd)
	initRepmgrFlags(monitorCmd)
	monitorCmd.Flags().IntVar(&conf.MaxFail, "failcount", 5, "Trigger failover after N failures (interval 1s)")
	monitorCmd.Flags().Int64Var(&conf.FailResetTime, "failcount-reset-time", 300, "Reset failures counter after N seconds")
	monitorCmd.Flags().Int64Var(&conf.MonitoringTicker, "monitoring-ticker", 2, "Monitoring time interval in seconds")
	monitorCmd.Flags().BoolVar(&conf.Autorejoin, "autorejoin", true, "Automatically rejoin a failed server to the current master")
	monitorCmd.Flags().BoolVar(&conf.AutorejoinFlashback, "autorejoin-flashback", false, "Automatically rejoin a failed server to the current master and flashback at the time of election if new master was delayed")
	monitorCmd.Flags().BoolVar(&conf.AutorejoinSemisync, "autorejoin-semisync", true, "Automatically rejoin a failed server to the current master when elected semisync status is SYNC ")
	monitorCmd.Flags().BoolVar(&conf.AutorejoinMysqldump, "autorejoin-mysqldump", false, "Automatically rejoin a failed server to the current master using mysqldump")
	monitorCmd.Flags().BoolVar(&conf.AutorejoinBackupBinlog, "autorejoin-backup-binlog", true, "Automatically backup ahead binlogs when old master rejoin")
	monitorCmd.Flags().BoolVar(&conf.CheckFalsePositiveHeartbeat, "failover-falsepositive-heartbeat", true, "Failover checks that slaves do not receive hearbeat")
	monitorCmd.Flags().IntVar(&conf.CheckFalsePositiveHeartbeatTimeout, "failover-falsepositive-heartbeat-timeout", 3, "Failover checks that slaves do not receive hearbeat detection timeout ")
	monitorCmd.Flags().BoolVar(&conf.CheckFalsePositiveMaxscale, "failover-falsepositive-maxscale", false, "Failover checks that maxscale detect failed master")
	monitorCmd.Flags().IntVar(&conf.CheckFalsePositiveMaxscaleTimeout, "failover-falsepositive-maxscale-timeout", 14, "Failover checks that maxscale detect failed master")
	monitorCmd.Flags().BoolVar(&conf.CheckFalsePositiveExternal, "failover-falsepositive-external", false, "Failover checks that http//master:80 does not reponse 200 OK header")
	monitorCmd.Flags().IntVar(&conf.CheckFalsePositiveExternalPort, "failover-falsepositive-external-port", 80, "Failover checks external port")
	monitorCmd.Flags().BoolVar(&conf.ForceSlaveHeartbeat, "force-slave-heartbeat", false, "Automatically activate heartbeat on slave")
	monitorCmd.Flags().IntVar(&conf.ForceSlaveHeartbeatRetry, "force-slave-heartbeat-retry", 5, "Replication heartbeat retry on slave")
	monitorCmd.Flags().IntVar(&conf.ForceSlaveHeartbeatTime, "force-slave-heartbeat-time", 3, "Replication heartbeat time")
	monitorCmd.Flags().BoolVar(&conf.ForceSlaveGtid, "force-slave-gtid-mode", false, "Automatically activate gtid mode on slave")
	monitorCmd.Flags().BoolVar(&conf.ForceSlaveNoGtid, "force-slave-no-gtid-mode", false, "Automatically activate no gtid mode on slave")
	monitorCmd.Flags().BoolVar(&conf.ForceSlaveSemisync, "force-slave-semisync", false, "Automatically activate semisync on slave")
	monitorCmd.Flags().BoolVar(&conf.ForceSlaveReadOnly, "force-slave-readonly", false, "Automatically activate read only on slave")
	monitorCmd.Flags().BoolVar(&conf.ForceBinlogRow, "force-binlog-row", false, "Automatically activate binlog row format on master")
	monitorCmd.Flags().BoolVar(&conf.ForceBinlogAnnotate, "force-binlog-annotate", false, "Automatically activate annotate event")
	monitorCmd.Flags().BoolVar(&conf.ForceBinlogSlowqueries, "force-binlog-slowqueries", false, "Automatically activate long replication statement in slow log")
	monitorCmd.Flags().BoolVar(&conf.ForceBinlogChecksum, "force-binlog-checksum", false, "Automatically force  binlog checksum")
	monitorCmd.Flags().BoolVar(&conf.ForceBinlogCompress, "force-binlog-compress", false, "Automatically force binlog compression")
	monitorCmd.Flags().BoolVar(&conf.ForceDiskRelayLogSizeLimit, "force-disk-relaylog-size-limit", false, "Automatically limit the size of relay log on disk ")
	monitorCmd.Flags().Uint64Var(&conf.ForceDiskRelayLogSizeLimitSize, "force-disk-relaylog-size-limit-size", 1000000000, "Automatically limit the size of relay log on disk to 1G")
	monitorCmd.Flags().BoolVar(&conf.ForceInmemoryBinlogCacheSize, "force-inmemory-binlog-cache-size", false, "Automatically adapt binlog cache size based on monitoring")
	monitorCmd.Flags().BoolVar(&conf.ForceSyncBinlog, "force-sync-binlog", false, "Automatically force master crash safe")
	monitorCmd.Flags().BoolVar(&conf.ForceSyncInnoDB, "force-sync-innodb", false, "Automatically force master innodb crash safe")
	monitorCmd.Flags().BoolVar(&conf.ForceNoslaveBehind, "force-noslave-behind", false, "Automatically force no slave behing")
	monitorCmd.Flags().StringVar(&conf.CheckType, "check-type", "tcp", "Type of server health check (tcp, agent)")
	monitorCmd.Flags().BoolVar(&conf.CheckReplFilter, "check-replication-filters", true, "Check that elected master have equal replication filters")
	monitorCmd.Flags().BoolVar(&conf.CheckBinFilter, "check-binlog-filters", true, "Check that elected master have equal binlog filters")
	monitorCmd.Flags().BoolVar(&conf.RplChecks, "check-replication-state", true, "Ignore replication checks for failover purposes")
	monitorCmd.Flags().StringVar(&conf.ShareDir, "share-directory", "/usr/share/replication-manager", "Path to HTTP monitor share files")
	monitorCmd.Flags().StringVar(&conf.WorkingDir, "working-directory", "/var/lib/replication-manager", "Path to HTTP monitor working directory")
	monitorCmd.Flags().BoolVar(&conf.HttpServ, "http-server", false, "Start the HTTP monitor")
	monitorCmd.Flags().StringVar(&conf.BindAddr, "http-bind-address", "localhost", "Bind HTTP monitor to this IP address")
	monitorCmd.Flags().StringVar(&conf.HttpPort, "http-port", "10001", "HTTP monitor to listen on this port")
	monitorCmd.Flags().StringVar(&conf.HttpRoot, "http-root", "/usr/share/replication-manager/dashboard", "Path to HTTP monitor files")
	monitorCmd.Flags().BoolVar(&conf.HttpAuth, "http-auth", false, "Authenticate to web server")
	monitorCmd.Flags().BoolVar(&conf.HttpBootstrapButton, "http-bootstrap-button", false, "Deprecate for the test flag. Get a boostrap option to init replication")
	monitorCmd.Flags().IntVar(&conf.SessionLifeTime, "http-session-lifetime", 3600, "Http Session life time ")
	monitorCmd.Flags().StringVar(&conf.MailFrom, "mail-from", "mrm@localhost", "Alert email sender")
	monitorCmd.Flags().StringVar(&conf.MailTo, "mail-to", "", "Alert email recipients, separated by commas")
	monitorCmd.Flags().StringVar(&conf.MailSMTPAddr, "mail-smtp-addr", "localhost:25", "Alert email SMTP server address, in host:[port] format")
	monitorCmd.Flags().BoolVar(&conf.Daemon, "daemon", false, "Daemon mode. Do not start the Termbox console")
	monitorCmd.Flags().BoolVar(&conf.Interactive, "interactive", true, "Ask for user interaction when failures are detected")
	monitorCmd.Flags().BoolVar(&conf.MxsOn, "maxscale", false, "Synchronize replication status with MaxScale proxy server")
	monitorCmd.Flags().BoolVar(&conf.MxsBinlogOn, "maxscale-binlog", false, "Turn on maxscale binlog server detection")
	monitorCmd.Flags().MarkDeprecated("maxscale-monitor", "Deprecate disable maxscale monitoring for 2 nodes cluster")
	monitorCmd.Flags().BoolVar(&conf.MxsMonitor, "maxscale-disable-monitor", false, "Disable maxscale monitoring for 2 nodes cluster")
	monitorCmd.Flags().StringVar(&conf.MxsGetInfoMethod, "maxscale-get-info-method", "maxadmin", "How to get infos from Maxscale maxinfo|maxadmin")
	monitorCmd.Flags().StringVar(&conf.MxsHost, "maxscale-host", "127.0.0.1", "MaxScale host IP")
	monitorCmd.Flags().StringVar(&conf.MxsPort, "maxscale-port", "6603", "MaxScale admin port")
	monitorCmd.Flags().StringVar(&conf.MxsUser, "maxscale-user", "admin", "MaxScale admin user")
	monitorCmd.Flags().StringVar(&conf.MxsPass, "maxscale-pass", "mariadb", "MaxScale admin password")
	monitorCmd.Flags().IntVar(&conf.MxsWritePort, "maxscale-write-port", 3306, "MaxScale read-write port to leader")
	monitorCmd.Flags().IntVar(&conf.MxsReadPort, "maxscale-read-port", 3307, "MaxScale load balance read port to all nodes")
	monitorCmd.Flags().IntVar(&conf.MxsReadWritePort, "maxscale-read-write-port", 3308, "MaxScale load balance read port to all nodes")
	monitorCmd.Flags().IntVar(&conf.MxsMaxinfoPort, "maxscale-maxinfo-port", 3309, "MaxScale maxinfo plugin http port")
	monitorCmd.Flags().IntVar(&conf.MxsBinlogPort, "maxscale-binlog-port", 3309, "MaxScale maxinfo plugin http port")
	monitorCmd.Flags().BoolVar(&conf.MxsServerMatchPort, "maxscale-server-match-port", false, "Match servers running on same host with different port")
	monitorCmd.Flags().BoolVar(&conf.MdbsProxyOn, "mdbshardproxy", false, "Wrapper to use Spider MdbProxy ")
	monitorCmd.Flags().StringVar(&conf.MdbsProxyHosts, "mdbshardproxy-hosts", "127.0.0.1:3307", "MariaDB spider proxy hosts IP:Port,IP:Port")
	monitorCmd.Flags().StringVar(&conf.MdbsProxyUser, "mdbshardproxy-user", "admin", "MaxScale admin user")
	monitorCmd.Flags().BoolVar(&conf.HaproxyOn, "haproxy", false, "Wrapper to use HaProxy on same host")
	monitorCmd.Flags().IntVar(&conf.HaproxyWritePort, "haproxy-write-port", 3306, "HaProxy read-write port to leader")
	monitorCmd.Flags().IntVar(&conf.HaproxyReadPort, "haproxy-read-port", 3307, "HaProxy load balance read port to all nodes")
	monitorCmd.Flags().IntVar(&conf.HaproxyStatPort, "haproxy-stat-port", 1988, "HaProxy statistics port")
	monitorCmd.Flags().StringVar(&conf.HaproxyBinaryPath, "haproxy-binary-path", "/usr/sbin/haproxy", "HaProxy binary location")
	monitorCmd.Flags().StringVar(&conf.HaproxyReadBindIp, "haproxy-ip-read-bind", "0.0.0.0", "HaProxy input bind address for read")
	monitorCmd.Flags().StringVar(&conf.HaproxyWriteBindIp, "haproxy-ip-write-bind", "0.0.0.0", "HaProxy input bind address for write")
	monitorCmd.Flags().IntVar(&conf.GraphiteCarbonPort, "graphite-carbon-port", 2003, "Graphite Carbon Metrics TCP & UDP port")
	monitorCmd.Flags().IntVar(&conf.GraphiteCarbonApiPort, "graphite-carbon-api-port", 10002, "Graphite Carbon API port")
	monitorCmd.Flags().IntVar(&conf.GraphiteCarbonServerPort, "graphite-carbon-server-port", 10003, "Graphite Carbon HTTP port")
	monitorCmd.Flags().IntVar(&conf.GraphiteCarbonLinkPort, "graphite-carbon-link-port", 7002, "Graphite Carbon Link port")
	monitorCmd.Flags().IntVar(&conf.GraphiteCarbonPicklePort, "graphite-carbon-pickle-port", 2004, "Graphite Carbon Pickle port")
	monitorCmd.Flags().IntVar(&conf.GraphiteCarbonPprofPort, "graphite-carbon-pprof-port", 7007, "Graphite Carbon Pickle port")
	monitorCmd.Flags().StringVar(&conf.GraphiteCarbonHost, "graphite-carbon-host", "127.0.0.1", "Graphite monitoring host")
	monitorCmd.Flags().BoolVar(&conf.GraphiteMetrics, "graphite-metrics", false, "Enable Graphite monitoring")
	monitorCmd.Flags().BoolVar(&conf.GraphiteEmbedded, "graphite-embedded", false, "Enable Internal Graphite Carbon Server")
	monitorCmd.Flags().IntVar(&conf.SysbenchTime, "sysbench-time", 100, "Time to run benchmark")
	monitorCmd.Flags().IntVar(&conf.SysbenchThreads, "sysbench-threads", 4, "Number of threads to run benchmark")
	monitorCmd.Flags().StringVar(&conf.SysbenchBinaryPath, "sysbench-binary-path", "/usr/sbin/sysbench", "Sysbench Wrapper in test mode")
	monitorCmd.Flags().StringVar(&conf.MariaDBBinaryPath, "mariadb-binary-path", "/usr/sbin/", "MariaDB 10.2 Bindir for binary logs backup of ahead trx ")
	monitorCmd.Flags().MarkDeprecated("mariadb-binary-path", "mariadb-binary-path is deprecated, please replace by mariadb-mysqlbinlog-path")
	monitorCmd.Flags().StringVar(&conf.MariaDBBinaryPath, "mariadb-mysqlbinlog-path", "/usr/share/replication-manager/", "Path to MariaDB 10.2 mysqlbinlog for binary logs trx ahead backup")
	monitorCmd.Flags().BoolVar(&conf.Heartbeat, "heartbeat-table", false, "Heartbeat for active/passive or multi mrm setup")
	monitorCmd.Flags().BoolVar(&conf.Arbitration, "arbitration-external", false, "Multi moninitor sas arbitration")
	monitorCmd.Flags().StringVar(&conf.ArbitrationSasSecret, "arbitration-external-secret", "", "")
	monitorCmd.Flags().StringVar(&conf.ArbitrationSasHosts, "arbitration-external-hosts", "88.191.151.84:80", "")
	monitorCmd.Flags().IntVar(&conf.ArbitrationSasUniqueId, "arbitration-external-unique-id", 0, "Unique instance idententifier")
	monitorCmd.Flags().StringVar(&conf.ArbitrationPeerHosts, "arbitration-peer-hosts", "127.0.0.1:10002", "replication-manager hosts http port")
	monitorCmd.Flags().BoolVar(&conf.Enterprise, "enterprise", false, "Enterpise release")

	viper.BindPFlags(monitorCmd.Flags())

	viper.RegisterAlias("mariadb-binary-path", "mariadb-mysqlbinlog-path")

	var err error
	repmgrHostname, err = os.Hostname()
	if err != nil {
		log.Fatalln("ERROR: replication-manager could not get hostname from system")
	}
}

// initRepmgrFlags function is used to initialize flags that are common to several subcommands
// e.g. monitor, failover, switchover.
// If you add a subcommand that shares flags with other subcommand scenarios please call this function.
// If you add flags that impact all the possible scenarios please do it here.
func initRepmgrFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&conf.PreScript, "pre-failover-script", "", "Path of pre-failover script")
	cmd.Flags().StringVar(&conf.PostScript, "post-failover-script", "", "Path of post-failover script")
	cmd.Flags().StringVar(&conf.RejoinScript, "rejoin-script", "", "Path of old master rejoin script")
	cmd.Flags().StringVar(&conf.PrefMaster, "prefmaster", "", "Preferred candidate server for master failover, in host:[port] format")
	cmd.Flags().StringVar(&conf.IgnoreSrv, "ignore-servers", "", "List of servers to ignore in slave promotion operations")
	cmd.Flags().Int64Var(&conf.SwitchWaitKill, "wait-kill", 5000, "Deprecate for switchover-wait-kill Wait this many milliseconds before killing threads on demoted master")
	cmd.Flags().Int64Var(&conf.SwitchWaitKill, "switchover-wait-kill", 5000, "Wait this many milliseconds before killing threads on demoted master")
	cmd.Flags().Int64Var(&conf.SwitchWaitTrx, "wait-trx", 10, "Depecrate for switchover-wait-trx Wait this many seconds before transactions end to cancel switchover")
	cmd.Flags().Int64Var(&conf.SwitchWaitTrx, "switchover-wait-trx", 10, "Wait this many seconds before transactions end to cancel switchover")
	cmd.Flags().BoolVar(&conf.SwitchSync, "switchover-at-sync", false, "Only failover when state semisync is sync for last status")
	cmd.Flags().BoolVar(&conf.SwitchGtidCheck, "gtidcheck", false, "Depecrate for failover-at-equal-gtid do not initiate failover unless slaves are fully in sync")
	cmd.Flags().BoolVar(&conf.SwitchGtidCheck, "switchover-at-equal-gtid", false, "Do not initiate failover unless slaves are fully in sync")
	cmd.Flags().Int64Var(&conf.SwitchMaxDelay, "switchover-max-slave-delay", 30, "Maximum replication delay before initiating switchover (-1 = unlimited)")

	cmd.Flags().IntVar(&conf.SwitchWaitWrite, "wait-write-query", 10, "Deprecate  Wait this many seconds before write query end to cancel switchover")
	cmd.Flags().IntVar(&conf.SwitchWaitWrite, "switchover-wait-write-query", 10, "Wait this many seconds before write query end to cancel switchover")

	cmd.Flags().BoolVar(&conf.ReadOnly, "readonly", true, "Set slaves as read-only after switchover")
	cmd.Flags().StringVar(&conf.LogFile, "logfile", "", "Write MRM messages to a log file")
	cmd.Flags().IntVar(&conf.Timeout, "connect-timeout", 5, "Database connection timeout in seconds")
	cmd.Flags().IntVar(&conf.ReadTimeout, "read-timeout", 15, "Database read timeout in seconds")
	cmd.Flags().StringVar(&conf.MasterConn, "master-connection", "", "Connection name to use for multisource replication")
	cmd.Flags().BoolVar(&conf.MultiMaster, "multimaster", false, "Turn on multi-master detection")
	cmd.Flags().BoolVar(&conf.MultiTierSlave, "multi-tier-slave", false, "Turn on to enable relay slaves in the topology")
	cmd.Flags().BoolVar(&conf.Spider, "spider", false, "Turn on spider detection")
	cmd.Flags().BoolVar(&conf.Test, "test", false, "Enable non regression tests ")
	cmd.Flags().Int64Var(&conf.FailMaxDelay, "maxdelay", 30, "(deprecated) Maximum replication delay before initiating failover")
	cmd.Flags().Int64Var(&conf.FailMaxDelay, "failover-max-slave-delay", 30, "Maximum replication delay before initiating failover (-1 = unlimited)")
	cmd.Flags().BoolVar(&conf.FailRestartUnsafe, "failover-restart-unsafe", false, "Failover when cluster down if a slave is start first ")
	cmd.Flags().IntVar(&conf.FailLimit, "failover-limit", 5, "Quit monitor after N failovers (0: unlimited)")
	cmd.Flags().Int64Var(&conf.FailTime, "failover-time-limit", 0, "In automatic mode, Wait N seconds before attempting next failover (0: do not wait)")
	cmd.Flags().BoolVar(&conf.FailSync, "failover-at-sync", false, "Only failover when state semisync is sync for last status")
	cmd.Flags().BoolVar(&conf.FailEventScheduler, "failover-event-scheduler", false, "Failover Event Scheduler")
	cmd.Flags().BoolVar(&conf.FailEventStatus, "failover-event-status", false, "Failover Event Status ENABLE OR DISABLE ON SLAVE")
	cmd.Flags().IntVar(&conf.MasterConnectRetry, "master-connect-retry", 10, "Specifies how many seconds to wait between slave connect retries to master")

	cmd.Flags().BoolVar(&conf.ReplicationSSL, "replication-use-ssl", false, "Use SSL encryption to replicate from master")
	viper.BindPFlags(cmd.Flags())

}

var failoverCmd = &cobra.Command{
	Use:   "failover",
	Short: "Failover a dead master",
	Long:  `Trigger failover on a dead master by promoting a slave.`,
	Run: func(cmd *cobra.Command, args []string) {

		currentCluster = new(cluster.Cluster)
		tlog := termlog.TermLog{}
		err := currentCluster.Init(conf, cfgGroup, &tlog, termlength, runUUID, Version, repmgrHostname, nil)
		if err != nil {
			log.WithError(err).Fatal("Error initializing cluster")
		}
		currentCluster.SetLogStdout()
		currentCluster.TopologyDiscover()
		currentCluster.FailoverForce()

	},
	PostRun: func(cmd *cobra.Command, args []string) {
		// Close connections on exit.
		currentCluster.Close()
	},
}

var switchoverCmd = &cobra.Command{
	Use:   "switchover",
	Short: "Perform a master switch",
	Long: `Performs an online master switch by promoting a slave to master
and demoting the old master to slave`,
	Run: func(cmd *cobra.Command, args []string) {
		currentCluster = new(cluster.Cluster)
		tlog := termlog.TermLog{}
		err := currentCluster.Init(confs[cfgGroup], cfgGroup, &tlog, termlength, runUUID, Version, repmgrHostname, nil)
		if err != nil {
			log.WithError(err).Fatal("Error initializing cluster")
		}
		currentCluster.SetLogStdout()
		currentCluster.TopologyDiscover()
		time.Sleep(time.Millisecond * 3000)
		currentCluster.MasterFailover(false)
	},
	PostRun: func(cmd *cobra.Command, args []string) {
		// Close connections on exit.
		currentCluster.Close()
	},
}

var topologyCmd = &cobra.Command{
	Use:   "topology",
	Short: "Print replication topology",
	Long:  `Print the replication topology by detecting master and slaves`,
	Run: func(cmd *cobra.Command, args []string) {
		currentCluster = new(cluster.Cluster)
		err := currentCluster.Init(confs[cfgGroup], cfgGroup, &termlog.TermLog{}, termlength, runUUID, Version, repmgrHostname, nil)
		if err != nil {
			log.WithError(err).Fatal("Error initializing cluster")
		}
		currentCluster.SetLogStdout()
		currentCluster.TopologyDiscover()
		currentCluster.PrintTopology()

	},
	PostRun: func(cmd *cobra.Command, args []string) {
		// Close connections on exit.
		currentCluster.Close()
	},
}

var monitorCmd = &cobra.Command{
	Use:   "monitor",
	Short: "Starts the interactive replication monitor",
	Long: `Starts replication-manager in stateful monitor daemon mode.
Interactive console and HTTP dashboards are available for control`,
	Run: func(cmd *cobra.Command, args []string) {

		if conf.LogLevel > 1 {
			log.SetLevel(log.DebugLevel)
		}
		if conf.Arbitration == true {
			runStatus = "S"
		}
		if !conf.Daemon {
			err := termbox.Init()
			if err != nil {
				log.WithError(err).Fatal("Termbox initialization error")
			}
		}
		if conf.Daemon {
			termlength = 120
			log.WithField("version", Version).Info("replication-manager started in daemon mode")
		} else {
			_, termlength = termbox.Size()
			if termlength == 0 {
				termlength = 120
			} else if termlength < 18 {
				log.Fatal("Terminal too small, please increase window size")
			}
		}
		loglen := termlength - 9 - (len(strings.Split(conf.Hosts, ",")) * 3)
		tlog = termlog.NewTermLog(loglen)
		// Initialize go-carbon

		if conf.Enterprise {

			var svc opensvc.Collector
			svc.Host = "127.0.0.1"
			svc.Port = "443"
			svc.User = "root@localhost.localdomain"
			svc.Pass = "opensvc"
			svc.RplMgrUser = "replication-manager"
			svc.RplMgrPassword = "mariadb"

			err := svc.Bootstrap(conf.ShareDir + "/opensvc/moduleset_mariadb.svc.mrm.db.cnf.json")
			if err != nil {
				log.Printf("%s", err)

			}
			agents = svc.GetNodes()

			//log.Println(agents)
		}
		if conf.GraphiteEmbedded {
			go graphite.RunCarbon(conf.ShareDir, conf.WorkingDir, conf.GraphiteCarbonPort, conf.GraphiteCarbonLinkPort, conf.GraphiteCarbonPicklePort, conf.GraphiteCarbonPprofPort, conf.GraphiteCarbonServerPort)
			log.WithFields(log.Fields{
				"metricport": conf.GraphiteCarbonPort,
				"httpport":   conf.GraphiteCarbonServerPort,
			}).Info("Carbon server started")

			/*
				carbonServer string host:port
				carbonApiPort int
				cacheType  default "mem"  "cache type to use"
				mc default "" "comma separated memcached server list"
				memsize int default 0 "in-memory cache size in MB (0 is unlimited)"
				cpus int default 0 "number of CPUs to use"
				tz string default "" "timezone,offset to use for dates with no timezone"
				logdir string "logging directory"
			*/

			time.Sleep(2 * time.Second)
			go graphite.RunCarbonApi("http://0.0.0.0:"+strconv.Itoa(conf.GraphiteCarbonServerPort), conf.GraphiteCarbonApiPort, 20, "mem", "", 200, 0, "", conf.WorkingDir)
			log.WithField("apiport", conf.GraphiteCarbonApiPort).Info("Carbon server API started")
		}

		// If there's an existing encryption key, decrypt the passwords
		k, err := readKey()
		if err != nil {
			log.WithError(err).Info("No existing password encryption scheme")
			k = nil
		}
		for _, gl := range cfgGroupList {
			currentCluster = new(cluster.Cluster)
			currentCluster.Init(confs[gl], gl, &tlog, termlength, runUUID, Version, repmgrHostname, k)
			clusters[gl] = currentCluster
			go currentCluster.Run()

		}
		currentCluster.SetCfgGroupDisplay(cfgGroup)

		// HTTP server should start after Cluster Init or may lead to various nil pointer if clients still requesting
		if conf.HttpServ {
			go httpserver()
		}

		termboxChan := newTbChan()
		interval := time.Second
		ticker := time.NewTicker(interval * time.Duration(conf.MonitoringTicker))
		for exit == false {

			select {
			case <-ticker.C:
				if conf.Arbitration {
					fHeartbeat()
				}
			case event := <-termboxChan:
				switch event.Type {
				case termbox.EventKey:
					if event.Key == termbox.KeyCtrlS {
						if currentCluster.IsMasterFailed() == false || currentCluster.GetMasterFailCount() > 0 {
							currentCluster.MasterFailover(false)
						} else {
							currentCluster.LogPrintf("ERROR", "Master failed, cannot initiate switchover")
						}
					}
					if event.Key == termbox.KeyCtrlF {
						if currentCluster.IsMasterFailed() {
							currentCluster.MasterFailover(true)
						} else {
							currentCluster.LogPrintf("ERROR", "Master not failed, cannot initiate failover")
						}
					}
					if event.Key == termbox.KeyCtrlD {
						currentCluster.PrintTopology()
					}
					if event.Key == termbox.KeyCtrlN {
						cfgGroupIndex++
						if cfgGroupIndex >= len(cfgGroupList) {
							cfgGroupIndex = 0
						}
						cfgGroup = cfgGroupList[cfgGroupIndex]
						currentCluster = clusters[cfgGroup]
						for _, gl := range cfgGroupList {
							clusters[gl].SetCfgGroupDisplay(cfgGroup)
						}
					}
					if event.Key == termbox.KeyCtrlP {
						cfgGroupIndex--
						if cfgGroupIndex < 0 {
							cfgGroupIndex = len(cfgGroupList) - 1
						}
						cfgGroup = cfgGroupList[cfgGroupIndex]
						currentCluster = clusters[cfgGroup]
						for _, gl := range cfgGroupList {
							clusters[gl].SetCfgGroupDisplay(cfgGroup)
						}
					}
					if event.Key == termbox.KeyCtrlR {
						currentCluster.LogPrintf("INFO", "Setting slaves read-only")
						currentCluster.SetSlavesReadOnly(true)
					}
					if event.Key == termbox.KeyCtrlW {
						currentCluster.LogPrint("INFO", "Setting slaves read-write")
						currentCluster.SetSlavesReadOnly(false)
					}
					if event.Key == termbox.KeyCtrlI {
						currentCluster.ToggleInteractive()
					}
					if event.Key == termbox.KeyCtrlH {
						currentCluster.DisplayHelp()
					}
					if event.Key == termbox.KeyCtrlQ {
						currentCluster.LogPrintf("INFO", "Quitting monitor")
						exit = true
						currentCluster.Stop()
					}
					if event.Key == termbox.KeyCtrlC {
						currentCluster.LogPrintf("INFO", "Quitting monitor")
						exit = true
						currentCluster.Stop()
					}
				}
				switch event.Ch {
				case 's':
					termbox.Sync()
				}
			}

		}
		if exitMsg != "" {
			log.Println(exitMsg)
		}
	},
	PostRun: func(cmd *cobra.Command, args []string) {
		// Close connections on exit.
		currentCluster.Close()
		termbox.Close()
		if memprofile != "" {
			f, err := os.Create(memprofile)
			if err != nil {
				log.Fatal(err)
			}
			pprof.WriteHeapProfile(f)
			f.Close()
		}
	},
}

func newTbChan() chan termbox.Event {
	termboxChan := make(chan termbox.Event)
	go func() {
		for {
			termboxChan <- termbox.PollEvent()
		}
	}()
	return termboxChan
}
