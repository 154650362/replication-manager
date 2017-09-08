// replication-manager - Replication Manager Monitoring and CLI for MariaDB and MySQL
// Authors: Guillaume Lefranc <guillaume@signal18.io>
//          Stephane Varoqui  <stephane@mariadb.com>
// This source code is licensed under the GNU General Public License, version 3.

package regtest

import "github.com/tanji/replication-manager/cluster"

func testFailoverAllSlavesDelayRplChecksNoSemiSync(cluster *cluster.Cluster, conf string, test *cluster.Test) bool {

	err := cluster.DisableSemisync()
	if err != nil {
		cluster.LogPrintf("ERROR", "%s", err)
		return false
	}

	SaveMasterURL := cluster.GetMaster().URL
	cluster.LogPrintf("TEST", "Master is %s", cluster.GetMaster().URL)
	cluster.SetInteractive(false)
	cluster.SetCheckFalsePositiveHeartbeat(false)
	cluster.SetRplChecks(true)
	cluster.SetRplMaxDelay(4)
	cluster.SetFailoverCtr(1)
	cluster.DelayAllSlaves()
	cluster.FailoverNow()
	cluster.LogPrintf("TEST", " New Master  %s ", cluster.GetMaster().URL)
	if cluster.GetMaster().URL != SaveMasterURL {
		cluster.LogPrintf("ERROR", "Old master %s !=  New master %s  ", SaveMasterURL, cluster.GetMaster().URL)
		return false
	}

	return true
}
