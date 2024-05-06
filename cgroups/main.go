package main

import (
	"log"
	"time"
	"os/exec"
	"github.com/containerd/cgroups/v3/cgroup2"
)

func pointerInt64(int int64) *int64 {
	return &int
}

func main() {
	var (
		quota  int64  = 200000
		period uint64 = 1000000
		//weight uint64 = 100
		maj  int64  = 8
		min  int64  = 0
		rate uint64 = 120
		max int64 = 1000
	)
	res := cgroup2.Resources{
		CPU: &cgroup2.CPU{
			//Weight: &weight, // e.g. (weight in the child cgroup) / (sum of cpu weights in the control groups) => percentage of cpu for this child cgroup processes
			Max:    cgroup2.NewCPUMax(&quota, &period), // e.g. 200000 1000000 meaning processes inside this cgroup can (together) run on the CPU for only 0.2 sec every 1 second
			//Cpus:   "0", // This limits on which CPU cores can the processes inside this cgroup run (NOTE: Also "Mems" needs to be set: https://github.com/containerd/cgroups/blob/fa6f6841ed3d57355acadbc06f1d7ed4d91ac4f7/cgroup2/manager.go#L97!)
			//Mems:   "0", // Memory Node” refers to an on-line node that contains memory. 
		},
		Memory: &cgroup2.Memory{
			Max:  pointerInt64(629145600), // ~629MB // If a cgroup's memory usage reaches this limit and can't be reduced, the system OOM killer is invoked on the cgroup. 
			Swap: pointerInt64(314572800), // Swap usage in bytes
			High: pointerInt64(524288000), // memory usage throttle limit. If a cgroup's memory use goes over the high boundary specified here, the cgroup’s processes are throttled and put under heavy reclaim pressure. The default is max, meaning there is no limit.
		},
		IO: &cgroup2.IO{
			Max: []cgroup2.Entry{{
				Major: maj, 
				Minor: min, 
				Type: cgroup2.ReadIOPS, // Limit I/O Read Operations per second for a block device identified as (major, minor) - e.g. "ls -l /dev/sda*"
				Rate: rate, // number of (read) operations per second
			}},
		},
		Pids: &cgroup2.Pids{
			Max: max, // number of processes allowed - The process number controller is used to allow a cgroup hierarchy to stop any new tasks from being fork()’d or clone()’d after a certain limit is reached.
		},
	}
	
	// dummy PID of -1 is used for creating a "general slice" to be used as a parent cgroup.
	// see https://github.com/containerd/cgroups/blob/1df78138f1e1e6ee593db155c6b369466f577651/v2/manager.go#L732-L735
	// "'-' inside the cgroup name make a child branch => my-cgroup-abc.slice - my.slice/my-cgroup.slice/my-cgroup-abc.slice/<processes>"
	m, err := cgroup2.NewSystemd("/", "my-cgroup-abc.slice", -1, &res)
	if err != nil {
		log.Fatalln(err)
	}
	cgType, err := m.GetType()
	if err != nil {
		log.Fatalln(err)
	}
	log.Println(cgType)

	cmd := exec.Command("stress", "-c", "1", "--timeout", "30")
	// Start the command
	if err := cmd.Start(); err != nil {
		log.Printf("Error starting the command: %v\n", err)
		return
	}

	// Retrieve the PID of the started process (+1 because stress command internally spawns another child process depending upon how many CPUs you want to stress (one process per stressed CPU))
	pid := cmd.Process.Pid + 1
	log.Printf("PID of the spawned process: %d\n", pid)


	if err := m.AddProc(uint64(pid)); err != nil {
		log.Fatalln(err)
	}

	procs, err := m.Procs(false)
	if err != nil {
		log.Fatalln(err)
	}
	log.Printf("List of processes inside this cgroup: %v", procs)

	log.Println("Freezing Process")
	if err := m.Freeze(); err != nil {
		log.Fatalln(err)
	}
	time.Sleep(time.Second * 15)

	log.Println("Thawing Process")
	if err := m.Thaw(); err != nil {
		log.Fatalln(err)
	}

	// Wait for the command to finish
	if err := cmd.Wait(); err != nil {
		log.Printf("Error waiting for the command to finish: %v\n", err)
		return
	}
	err = m.DeleteSystemd()
	if err != nil {
		log.Fatalln(err)
	}
}