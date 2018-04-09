package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	flag "github.com/spf13/pflag"
)

const (
	defaultPort = 8888
	defaultIP   = ""
	defaultRPS  = 1000
	jitter      = 50
)

// Simulation allows us to control simulations
type Simulation struct {
	NodeID        int
	ProduceErrors bool
	Pause         bool
	RPS           int
	Version       int
}

var (
	reqSummary = prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Name: "request_time_ms",
		Help: "Requests to the svc-sim service",
	},
		[]string{"node", "code", "error"})

	badCodes = []int{400, 401, 403, 404, 500, 501, 503}

	chans = []chan Simulation{}

	simulation = false

	port int
	ip   string

	simulate bool
	nodes    int
	rps      int
)

func init() {
	// Register Prometheus summary
	prometheus.MustRegister(reqSummary)

	// configuration flags
	flag.IntVar(&port, "port", defaultPort, "port to listen on")
	flag.StringVar(&ip, "ip", defaultIP, "IP for the sever")

	// Simulation flags
	flag.BoolVar(&simulate, "simulate", false, "simulate a set of nodes")
	flag.IntVar(&nodes, "nodes", 2, "nodes in the simulation")
	flag.IntVar(&rps, "rps", defaultRPS, "requests per second for the simulation")

	flag.Parse()
}

func getNode(r *http.Request) int {
	rn := rand.New(rand.NewSource(int64(time.Now().Nanosecond())))
	var num int

	vals := r.URL.Query()
	if _, ok := vals["node"]; !ok {
		return rn.Intn(nodes)
	}

	var err error
	num, err = strconv.Atoi(vals["node"][0])
	if err != nil {
		return rn.Intn(nodes)
	}

	return num
}

func main() {
	logrus.SetOutput(os.Stdout)
	fieldDefaults := logrus.Fields{
		"node": "node0",
	}

	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "healthy")
	})

	http.HandleFunc("/fail-node", func(w http.ResponseWriter, r *http.Request) {
		node := getNode(r)
		c := chans[node-1]
		sim := Simulation{
			NodeID:        node,
			ProduceErrors: true,
		}

		c <- sim
		fmt.Fprintf(w, "Failing requests to node %d", node)

		sim = Simulation{
			RPS: rps / (nodes - 1),
		}

		for i := 0; i < nodes; i++ {
			if i != node-1 {
				c = chans[i]
				sim.NodeID = i + 1
				c <- sim
			}
		}
	})

	http.HandleFunc("/remove-node", func(w http.ResponseWriter, r *http.Request) {
		node := getNode(r)
		c := chans[node-1]
		sim := Simulation{
			NodeID: node,
			Pause:  true,
		}

		c <- sim
		fmt.Fprintf(w, "Removed node %d from service", node)

		sim = Simulation{
			RPS: rps / (nodes - 1),
		}

		for i := 0; i < nodes; i++ {
			if i != node-1 {
				c = chans[i]
				sim.NodeID = i + 1
				c <- sim
			}
		}
	})

	http.HandleFunc("/fix-node", func(w http.ResponseWriter, r *http.Request) {
		node := getNode(r)

		sim := Simulation{
			RPS:           rps / (nodes),
			Pause:         false,
			ProduceErrors: false,
		}

		for i := 0; i < nodes; i++ {
			c := chans[i]
			sim.NodeID = i + 1
			c <- sim
		}
		fmt.Fprintf(w, "Fixed node %d and put into service", node)
	})

	http.HandleFunc("/deploy", func(w http.ResponseWriter, r *http.Request) {
		deployTime := time.Duration(60)

		simPause := Simulation{
			RPS:   rps / (nodes - 1),
			Pause: true,
		}
		simResume := Simulation{
			RPS:   rps / nodes,
			Pause: false,
		}

		simTakeOver := Simulation{
			RPS:   rps / (nodes - 1),
			Pause: false,
		}

		for i := 0; i < nodes; i++ {
			for j := 0; j < nodes; j++ {
				if j == i {
					chans[j] <- simPause
				} else {
					chans[j] <- simTakeOver
				}
			}
			time.Sleep(deployTime * time.Second)

			for j := 0; j < nodes; j++ {
				chans[j] <- simResume
			}

			time.Sleep(15 * time.Second)
		}
	})

	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		// Let's track how long it will take
		start := time.Now()

		// Do actual work here
		fmt.Fprintf(w, "Hello")

		// Log and observe metrics
		dur := time.Since(start).Seconds()
		logrus.WithFields(fieldDefaults).WithFields(logrus.Fields{
			"code":     200,
			"isError":  false,
			"clientIP": r.RemoteAddr,
			"method":   r.Method,
			"path":     "/hello",
			"duration": dur,
		}).Info()
		reqSummary.WithLabelValues("node0", "200", "false").Observe(dur)
	})

	if simulate {
		logrus.Infof("Starting %d nodes", nodes)
		reqs := rps / nodes
		for i := 1; i <= nodes; i++ {
			sim := make(chan Simulation)
			logrus.Infof("Starting node %d", i)
			chans = append(chans, sim)
			go addReqs(i, reqs, sim)
		}
	}

	listenConfig := fmt.Sprintf("%s:%d", ip, port)
	logrus.WithFields(fieldDefaults).Infof("Starting server on port %s", listenConfig)
	log.Fatal(http.ListenAndServe(listenConfig, nil))
}

func addReqs(node, rps int, control chan Simulation) {
	r := rand.New(rand.NewSource(int64(time.Now().Nanosecond())))

	var (
		isError bool
		code    int
		rJitter int

		produceErrors = false
		paused        = false
	)
	timeout := time.NewTimer(1 * time.Second)

	go func() {
		for {
			sim := <-control

			if sim.ProduceErrors {
				produceErrors = sim.ProduceErrors
				code = 500
			} else {
				produceErrors = sim.ProduceErrors
			}

			if sim.Pause {
				paused = sim.Pause
			} else {
				paused = sim.Pause
			}

			rps = sim.RPS
		}
	}()

	for {
		timeout.Reset(1 * time.Second)
		rJitter = r.Intn(jitter)
		for i := 0; i < rps+rJitter && !paused; i++ {
			if !produceErrors {
				isError = false
				code = 200

				if r.Intn(10000) == 5 {
					position := r.Intn(len(badCodes))
					code = badCodes[position]
				}

				if code >= 500 {
					isError = true
				}
			} else {
				isError = true
			}

			latency := (r.Float64() * 200.0) + (r.Float64() * 75.0)
			reqSummary.WithLabelValues(fmt.Sprintf("node%d", node), fmt.Sprintf("%d", code), fmt.Sprintf("%t", isError)).Observe(latency)
		}
		<-timeout.C
	}
}
