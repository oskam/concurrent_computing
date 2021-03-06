// Author: Magdalena Oska 221492

package main

import (
	"bufio"
	"fmt"
	"math"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Train type represents train object with set id, speed, pasangers capacity,
// route marked by switches, also saves current switch index and position.
type Train struct {
	id, maxSpeed, capacity, repairDuration int
	route                                  []*Switch
	routeIndex                             int
	position                               BasicRail // rail on which train travels or stations
	stops                                  []*Station
	passengers                             []*Worker
	pendingPassengers                      map[*Station][]*Worker
	pendingLock                            sync.Mutex
	offRail                                chan bool // holds true when train changes rail to free previous
	repaired                               chan bool // holds true when train got repaired after break
}

// wait stops execution for s fraction of simulation hour calulated to seconds
func (t *Train) wait(s float64) {
	time.Sleep(time.Duration(s*float64(secondsInHour)) * time.Second)
}

// repairTime returns how long repair of t should take
func (t *Train) repairTime() float64 {
	return float64(t.repairDuration) / 60.0
}

func (t *Train) String() string {
	return "Train" + strconv.Itoa(t.id)
}

// Run is main Train function, that moves train on it's route
func (t *Train) Run(switches *[]*Switch, railway *[][][]BasicRail, repairRequest *chan interface{}, group *sync.WaitGroup) {
	defer group.Done()
	// place train on first Switch in route
	t.position = t.route[t.routeIndex]
	t.route[t.routeIndex].userTrain <- t
	<-t.offRail
	<-t.route[t.routeIndex].readyToGo
	// then in infinite loop move it onto rail, switch, rail, switch ...
	for {
		// get id of switch that is before the rail we have to get on
		from := t.route[t.routeIndex].id
		// get id of switch that is after the rail we have to get on
		to := t.route[(t.routeIndex+1)%len(t.route)].id
	FindRail:
		// loop until we find free rail to move onto
		for {
			// loop all rails between switches got above
			for _, r := range (*railway)[from][to] {
				// try to move onto rail
				switch r.(type) {
				case *Rail:
					r := r.(*Rail)
					select {
					case r.userTrain <- t:
						// when train enters rail successfully
						// wait for information from rail that it did its job
						<-r.readyToGo
						break FindRail
					default:
						// when it failed, continue to next one
						continue
					}
				case *Platform:
					r := r.(*Platform)
					select {
					case r.userTrain <- t:
						// when train enters rail successfully
						// wait for information from rail that it did its job
						<-r.readyToGo
						break FindRail
					default:
						// when it failed, continue to next one
						continue
					}
				}
			}
		}

		// with probability of 0.5% break (if repair train is ready)
		if 0.005 > rand.Float64() {
			select {
			case *repairRequest <- t:
				// when repair train can accept request
				// wait for information from repair train that train is fixed
				<-t.repaired
				if printInformation {
					fmt.Printf("%s\t%s is repaired\n",
						simulationNow(),
						t.String())
				}
			default:
				// when no repair train can accept request, act like nothing happened
				continue
			}
		}

		// we have to change route index (switch)
		t.routeIndex = (t.routeIndex + 1) % len(t.route)
		// enter next switch
		t.route[t.routeIndex].userTrain <- t
		<-t.route[t.routeIndex].readyToGo
	}
}

// RepairTrain type represents repair train object with set id, speed, depot, switch next to depot that is start of every route. It accepts request for fixing broken railway elements in infinite loop
type RepairTrain struct {
	id, maxSpeed int
	depot        *Platform         // where repair trains waits for requests, it have to come back to depot before accepting next request
	start        *Switch           // switch that depot is connected to
	offRail      chan bool         // holds true when repair train changes rail to free previous
	toRepair     *chan interface{} // pointer to global repair request channel
}

func (rt *RepairTrain) String() string { return "RepairTrain" + strconv.Itoa(rt.id) }

// Run is main RepairTrain function, that accepts repair requests, repairs, returns to depot in loop
func (rt *RepairTrain) Run(switches *[]*Switch, railway *[][][]BasicRail) {
	// place repair train in depot
	rt.depot.userRepairTrain <- rt
	<-rt.offRail
	<-rt.depot.readyToGo

	// then in infinite loop repair stuff
Loop1:
	for {
		// block until there is something to repair
		broken := <-*rt.toRepair

		// Get data
		var neighbors []BasicRail      // holds rails adjacent to broken one, repair train needs to enter one of them to perform repair
		var repairTime float64         // how long repair takes in simulation hour fraction
		var repairedChannel *chan bool // pointer to broken's channel that will inform about repair end
		var reservations []BasicRail   // all rails that got reserved

		switch broken.(type) {
		case BasicRail:
			rail := broken.(BasicRail)
			if printInformation {
				fmt.Printf("%s\t%s broke\n",
					simulationNow(),
					rail.String())
			}
			neighbors = rail.connections()
			repairTime = rail.repairTime()
			repairedChannel = rail.repairedChannel()

			// Check if Switch connected to depot is broken, then no need to move repair train
			if rt.start == broken {
				if printInformation {
					fmt.Printf("%s\t%s repairs %s for next %.2fh\n",
						simulationNow(),
						rt.String(),
						rail.String(),
						repairTime)
				}
				// pause goroutine for calculated time
				time.Sleep(time.Duration(repairTime*float64(secondsInHour)) * time.Second)
				*repairedChannel <- true
				goto Loop1
			}

		case *Train:
			train := broken.(*Train)

			if printInformation {
				fmt.Printf("%s\t%s broke at %s\n",
					simulationNow(),
					train.String(),
					train.position.String())
			}

			neighbors = train.position.connections()
			repairTime = train.repairTime()
			repairedChannel = &train.repaired
		}

		// Reservations
		reservations = make([]BasicRail, 0)
		// try to reserve every switch but broken one (if it is switch)
		for _, s := range *switches {
			if s == broken {
				// skip if it is broken
				continue
			}
			select {
			case s.suspended <- true:
				// if suspending switch was successful add it to list
				reservations = append(reservations, s)
				// set flag for route finding algorithm
				s.reservation <- true
			default:
				continue
			}
		}

		for i := range *railway {
			// try to reserve every rail and platform but broken one
		Loop:
			for j := 0; j <= i; j++ {
				for _, br := range (*railway)[i][j] {
					if br == broken {
						// skip if it is broken
						continue
					}
					switch br.(type) {
					case *Rail:
						rail := br.(*Rail)
						select {
						case rail.suspended <- true:
							// if suspending rail was successful add it to list
							reservations = append(reservations, rail)
							// set flag for route finding algorithm
							rail.reservation <- true
							continue Loop
						default:
							continue
						}
					case *Platform:
						platform := br.(*Platform)
						select {
						case platform.suspended <- true:
							// if suspending platform was successful add it to list
							reservations = append(reservations, platform)
							// set flag for route finding algorithm
							platform.reservation <- true
							continue Loop
						default:
							continue
						}
					}
				}
			}
		}

		// Find route to broken
		// make channel that will block until route is found
		routeChannel := make(chan []BasicRail)
		// start goroutine that will search for route to broken through reserved rails, starting from depot
		go findRoute([]BasicRail{rt.depot}, neighbors, &routeChannel)
		// when route is found take it off channel
		route := <-routeChannel

		if printInformation {
			for _, r := range route {
				fmt.Printf("%s\t%s reserved %s for route\n",
					simulationNow(),
					rt.String(),
					r.String())
			}
		}

		// Release unused reservations
	Loop2:
		for _, res := range reservations {
			for _, r := range route {
				// check if it is on route, skip if we need it
				if res == r {
					continue Loop2
				}
			}
			res.release()
		}

		// Take route to broken neighbor, start from index 1, as repair train is already in depot
		for _, r := range route[1:] {
			switch r.(type) {
			case *Platform:
				p := r.(*Platform)
				p.userRepairTrain <- rt
				<-p.readyToGo
			case *Rail:
				r := r.(*Rail)
				r.userRepairTrain <- rt
				<-r.readyToGo
			case *Switch:
				s := r.(*Switch)
				s.userRepairTrain <- rt
				<-s.readyToGo
			}
			// release every used rail
			r.release()
		}

		// Repair broken
		if printInformation {
			fmt.Printf("%s\t%s repairs %s for next %.2fh\n",
				simulationNow(),
				rt.String(),
				broken,
				repairTime)
		}
		// pause goroutine for calculated time
		time.Sleep(time.Duration(repairTime*float64(secondsInHour)) * time.Second)
		// inform broken that it got fixed
		*repairedChannel <- true

		// Return to depot following route backwards
		for i := len(route) - 2; i >= 0; i-- {
			r := route[i]
			switch r.(type) {
			case *Platform:
				// if it is platform use any that connects the same switches
				p := r.(*Platform)

				if p == rt.depot {
					// if platform is depot, just enter it
					p.userRepairTrain <- rt
					<-p.readyToGo
					continue
				}
				// get switches that platform connects
				neighbors := p.connections()
			FindPlatform:
				// loop until we find free platform to move onto
				for {
					// try every platform between neighbor switches
					for _, p := range (*railway)[neighbors[0].Id()][neighbors[1].Id()] {
						p := p.(*Platform)
						select {
						case p.userRepairTrain <- rt:
							<-p.readyToGo
							break FindPlatform
						default:
							continue
						}
					}
				}
			case *Rail:
				// if it is rail use any that connects the same switches
				r := r.(*Rail)
				// get switches that rail connects
				neighbors := r.connections()
			FindRail:
				// loop until we find free rail to move onto
				for {
					// try every rail between neighbor switches
					for _, r := range (*railway)[neighbors[0].Id()][neighbors[1].Id()] {
						r := r.(*Rail)
						select {
						case r.userRepairTrain <- rt:
							<-r.readyToGo
							break FindRail
						default:
							continue
						}
					}
				}
			case *Switch:
				// if it is switch, wait until its free, than use it
				s := r.(*Switch)
				s.userRepairTrain <- rt
				<-s.readyToGo
			}
		}
	}
}

// findRoute searches for route to broken railway element.
func findRoute(
	route []BasicRail, // current route
	ends []BasicRail, // set of rails that we try to reach
	finalRouteChan *chan []BasicRail, // channel that waits for final route
) {
Loop1:
	for _, track := range route[len(route)-1].connections() {
		// for every track current route end is connected to
		if track.reserved() {
			// if it is reserved
			for _, end := range ends {
				// check if it's one of destinations
				if track == end {
					// try to return found route
					select {
					case *finalRouteChan <- append(route, track):
						return
					default:
						// if channel can't accept route it means one was already found, return
						return
					}
				}
			}
			// if it is not end, extend route and continue search
			go findRoute(append(route, track), ends, finalRouteChan)
		} else {
			// if track is not reserved, try next one
			continue Loop1
		}
	}
}

// BasicRail is interface for all rail types: Rail, Platform, Switch
type BasicRail interface {
	waitTime(train interface{}) float64 // waitTime returns how long rail have to wait before train can leave it
	connections() []BasicRail           // connections returns rail neighbors
	repairTime() float64                // returns how long repair of rail should take
	repairedChannel() *chan bool        // returns pointer to channel that waits for information about successful repair
	reserved() bool                     // returns true if rails is reserved
	release()                           // cancels reservation releasing rail from suspension, removes reservation flag
	Id() int                            // return rail id
	String() string                     // returns string representation of rail
}

// Rail is basic rail for transport
type Rail struct {
	id, len, speedLimit, repairDuration int
	connects                            []BasicRail       // rails that are connected to this one
	userTrain                           chan *Train       // receives trains that want to enter rail
	userRepairTrain                     chan *RepairTrain // receives repair trains that want to enter rail
	readyToGo                           chan bool         // informs that rails did it's job
	repaired                            chan bool         // holds true when train got repaired after break
	suspended                           chan bool         // informs rail that it should suspend work
	released                            chan bool         // informs about suspension release
	reservation                         chan bool         // holds true when rail was successfully reserved
}

// Platform is rail on platform or in depot
type Platform struct {
	id, stopTime, repairDuration int
	connects                     []BasicRail       // rails that are connected to this one
	userTrain                    chan *Train       // receives trains that want to enter rail
	userRepairTrain              chan *RepairTrain // receives repair trains that want to enter rail
	readyToGo                    chan bool         // informs that rails did it's job
	repaired                     chan bool         // holds true when train got repaired after break
	suspended                    chan bool         // informs rail that it should suspend work
	released                     chan bool         // informs about suspension release
	reservation                  chan bool         // holds true when rail was successfully reserved
	station                      *Station
}

// Switch connects rails and enables train to move from one to another
type Switch struct {
	id, rotationTime, repairDuration int
	connects                         []BasicRail       // rails that are connected to this one
	userTrain                        chan *Train       // receives trains that want to enter rail
	userRepairTrain                  chan *RepairTrain // receives repair trains that want to enter rail
	readyToGo                        chan bool         // informs that rails did it's job
	repaired                         chan bool         // holds true when train got repaired after break
	suspended                        chan bool         // informs rail that it should suspend work
	released                         chan bool         // informs about suspension release
	reservation                      chan bool         // holds true when rail was successfully reserved
}

func (r *Rail) waitTime(train interface{}) float64 {
	switch train.(type) {
	case *Train:
		t := train.(*Train)
		return float64(r.len) / math.Min(float64(r.speedLimit), float64(t.maxSpeed))
	case *RepairTrain:
		rt := train.(*RepairTrain)
		return float64(r.len) / math.Min(float64(r.speedLimit), float64(rt.maxSpeed))
	default:
		panic("Wrong argument")
	}
}
func (p *Platform) waitTime(train interface{}) float64 {
	return float64(p.stopTime) / 60.0
}
func (s *Switch) waitTime(train interface{}) float64 {
	return float64(s.rotationTime) / 60.0
}

func (r *Rail) connections() []BasicRail {
	return r.connects
}
func (p *Platform) connections() []BasicRail {
	return p.connects
}
func (s *Switch) connections() []BasicRail {
	return s.connects
}

func (r *Rail) repairTime() float64 {
	return float64(r.repairDuration) / 60.0
}
func (p *Platform) repairTime() float64 {
	return float64(p.repairDuration) / 60.0
}
func (s *Switch) repairTime() float64 {
	return float64(s.repairDuration) / 60.0
}

func (r *Rail) repairedChannel() *chan bool {
	return &r.repaired
}
func (p *Platform) repairedChannel() *chan bool {
	return &p.repaired
}
func (s *Switch) repairedChannel() *chan bool {
	return &s.repaired
}

func (r *Rail) reserved() bool {
	select {
	case <-r.reservation:
		// if reservation flag was set, release it and return true
		return true
	default:
		return false
	}
}
func (p *Platform) reserved() bool {
	select {
	case <-p.reservation:
		// if reservation flag was set, release it and return true
		return true
	default:
		return false
	}
}
func (s *Switch) reserved() bool {
	select {
	case <-s.reservation:
		// if reservation flag was set, release it and return true
		return true
	default:
		return false
	}
}

func (r *Rail) release() {
	select {
	case <-r.reservation:
		// if reservation flag was set, release it before next repair
	default:
		break
	}
	select {
	case r.released <- true:
		// try to release rail if it was not yet released
		break
	default:
		break
	}
}
func (p *Platform) release() {
	select {
	case <-p.reservation:
		// if reservation flag was set, release it before next repair
	default:
		break
	}
	select {
	case p.released <- true:
		// try to release rail if it was not yet released
		break
	default:
		break
	}
}
func (s *Switch) release() {
	select {
	case <-s.reservation:
		// if reservation flag was set, release it before next repair
	default:
		break
	}
	select {
	case s.released <- true:
		// try to release rail if it was not yet released
		break
	default:
		break
	}
}

func (r *Rail) Id() int     { return r.id }
func (p *Platform) Id() int { return p.id }
func (s *Switch) Id() int   { return s.id }

func (r *Rail) String() string     { return "Rail" + strconv.Itoa(r.id) }
func (p *Platform) String() string { return "Platform" + strconv.Itoa(p.id) }
func (s *Switch) String() string   { return "Switch" + strconv.Itoa(s.id) }

// Ticket type represents planned workers travel
type Ticket struct {
	train *Train
	from  *Station
	to    *Station
}

// Task type represents job that workers have to do,
// it is stateful goroutine that waits for all workers presence
// before letting them know that they can do the task
type Task struct {
	location *Station
	time     int
	workers  map[*Worker]bool
	arrived  chan *Worker // channel on which workers report their presence
}

// Run is mai Task function, that waits for all workers and then informs them to start work
func (t *Task) Run(workers []*Worker) {
	// set all presence to false and notify workers about new task
	for _, w := range workers {
		t.workers[w] = false
		w.newTask <- t
	}
	// wait for workers
	for {
	Select:
		select {
		case worker := <-t.arrived:
			t.workers[worker] = true

			for _, ready := range t.workers {
				// if at least one worker is not ready, wait for next arrival
				if !ready {
					break Select
				}
			}
			// if all workers arrived, notify them
			for w := range t.workers {
				w.readyToWork <- true
			}
			// task done its job, it can return
			return
		}
	}
}

func (t *Task) taskTime() float64 {
	return float64(t.time) / 60.0
}

// Worker type represents railway worker that will wait for tasks,
// travel to their location, work and then return home
type Worker struct {
	id          int
	home        *Station
	position    *Station
	ticket      *Ticket
	task        *Task
	arrived     chan bool
	readyToWork chan bool
	newTask     chan *Task
}
// doTask locks until all Task workers are present, than waits for task time
func (w *Worker) doTask() {
	w.task.arrived <- w
	<-w.readyToWork

	if printInformation {
		fmt.Printf("%s\t%s is doing task at %s\n",
			simulationNow(),
			w.String(),
			w.task.location)
	}

	workTime := w.task.taskTime()
	time.Sleep(time.Duration(workTime*float64(secondsInHour)) * time.Second)

	if printInformation {
		fmt.Printf("%s\t%s ended task at %s\n",
			simulationNow(),
			w.String(),
			w.task.location)
	}

	w.task = nil
}
// getTicket simulates ticket purchase and locks until worker arrives at destination station
func (w *Worker) getTicket(train *Train, to *Station) {
	// get ticket
	w.ticket = &Ticket{
		train: train,
		from:  w.position,
		to:    to}
	// add it to train's tickets for departing station
	train.pendingLock.Lock()
	train.pendingPassengers[w.position] = append(train.pendingPassengers[w.position], w)
	train.pendingLock.Unlock()

	if printInformation {
		fmt.Printf("%s\t%s got ticket to %s for %s\n",
			simulationNow(),
			w.String(),
			to.String(),
			train.String())
	}
	// lock until arrived
	<-w.arrived
}

func (w *Worker) String() string { return fmt.Sprintf("Worker%d(%s)", w.id, w.home) }

// Station type represents station that have couple of platform, trains arriving at it and workers living here
type Station struct {
	id             int
	platforms      []*Platform
	inhabitants    []*Worker
	operatedTrains []*Train
}

func (s *Station) String() string {
	return fmt.Sprintf("Station%d", s.id)
}

// simulates clock in simulation time in format HH:MM
func simulationNow() string {
	// get duration from simulation start
	d := time.Since(startTime)
	// dividing real seconds by seconds per hour simulation
	// gives simulation hours, fractional is used to calculate minutes
	sH, f := math.Modf(d.Seconds() / float64(secondsInHour))
	// convert int to string representing simulation hours,
	// modulo 24 as day have 24 hours
	h := strconv.Itoa(int(sH) % 24)
	// if it is one digit, add leading 0
	if len(h) == 1 {
		h = "0" + h
	}
	// hour have 60 minutes, so multiplying fractional from
	// simulation hour calculation by 60 gives simulation minutes
	// take only integer part, f is in [0, 1) so result is in [0, 60)
	sM, _ := math.Modf(60.0 * f)
	// convert int to string representing simulation minutes
	m := strconv.Itoa(int(sM))
	// if it is one digit, add leading 0
	if len(m) == 1 {
		m = "0" + m
	}
	// return time in format HH:MM
	return strings.Join([]string{h, m}, ":")
}

// informant is function for goroutine that communicates with user
func informant(group *sync.WaitGroup, trains []*Train) {
	defer group.Done()

	s := "`i` for information, `e` to exit"
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Printf("%s: ", s)

		response, _ := reader.ReadString('\n')

		switch strings.ToLower(response)[0] {
		case 'i':
			fmt.Printf("Simulator clock: %s\n", simulationNow())
			for i := 0; i < len(trains); i++ {
				fmt.Printf("%s is at %s\n", trains[i].String(), trains[i].position.String())
			}
		case 'e':
			os.Exit(0)
		default:
			continue
		}
	}
}

// writeStat locks statistics file and then writes given string to it, unlocking afterwards
func writeStat(s string) {
	statMutex.Lock()
	defer statMutex.Unlock()

	stat.WriteString(s)
	stat.Flush()
}

var secondsInHour int                // how many seconds hour of simulation takes
var startTime time.Time              // simulation start time in real world
var stat *bufio.Writer               // writer for statistics
var statMutex sync.Mutex             // mutex locking writer to provide synchronization
var toRepairRequest chan interface{} // channel receiving requests for repair, every repair train have pointer to it which it checks for requests

var printInformation bool // flag defining operation mode
var filename string

func main() {
	startTime = time.Now()
	rand.Seed(startTime.UnixNano())

	if len(os.Args) != 3 {
		fmt.Println("need 2 arguments: <input filename [string]> <printInformation [bool]>")
		os.Exit(1)
	} else {
		filename = os.Args[1]
		b, err := strconv.ParseBool(os.Args[2])
		if err != nil {
			fmt.Println("second argument must be boolean")
			os.Exit(1)
		}
		printInformation = b
	}

	// create file for statistics, create writer
	f, err := os.Create("stats")
	if err != nil {
		panic(err)
	}
	stat = bufio.NewWriter(f)

	// open file in railway data, create scanner
	data, err := os.Open(filename)
	if err != nil {
		panic(err)
	}
	scanner := bufio.NewScanner(data)
	scanner.Scan()
	line := scanner.Text()
	fields := strings.Fields(line)

	// s - number of Switches defined
	s, _ := strconv.Atoi(fields[0])
	// p - number of Platforms defined
	p, _ := strconv.Atoi(fields[1])
	// r - number of Rails defined
	r, _ := strconv.Atoi(fields[2])
	// rt - number of RepairTrains defined
	rt, _ := strconv.Atoi(fields[3])
	// t - number of Trains defined
	t, _ := strconv.Atoi(fields[4])
	// w - number of Workers defined
	w, _ := strconv.Atoi(fields[5])
	// hour - number of seconds for hour simulation
	hour, _ := strconv.Atoi(fields[6])

	secondsInHour = hour
	toRepairRequest = make(chan interface{})

	// create arrays of pointers to switches, repair trains and trains, stations and workers
	switches := make([]*Switch, s)
	repairTrains := make([]*RepairTrain, rt)
	trains := make([]*Train, t)
	stations := make([]*Station, 0)
	workers := make([]*Worker, w)

	// create 3 dimensional array to keep railway graph represented by edges
	// connecting switches with id's of coordinates
	// railway[2][5] is array of rails between Switch 2 and Switch 5
	railway := make([][][]BasicRail, s)
	for i := range railway {
		railway[i] = make([][]BasicRail, s)
		for j := range railway[i] {
			railway[i][j] = []BasicRail{}
		}
	}

	// scan file for switches, create and save them in array switches
	for i := 0; i < s; i++ {
		scanner.Scan()
		line = scanner.Text()
		fields = strings.Fields(line)

		id, _ := strconv.Atoi(fields[0])
		rTime, _ := strconv.Atoi(fields[1])
		repDur, _ := strconv.Atoi(fields[2])

		switches[i] = &Switch{
			id:              id,
			rotationTime:    rTime,
			repairDuration:  repDur,
			connects:        make([]BasicRail, 0),
			userTrain:       make(chan *Train),
			userRepairTrain: make(chan *RepairTrain),
			readyToGo:       make(chan bool),
			repaired:        make(chan bool),
			suspended:       make(chan bool),
			released:        make(chan bool),
			reservation:     make(chan bool, 1)}

		go func(s *Switch, repairRequest *chan interface{}) {
			for {
				select {
				case rt := <-s.userRepairTrain:
					// when repair train want's to enter
					// inform last rail it is free
					rt.offRail <- true
					// calculate time we have to wait
					waitTime := s.waitTime(rt)
					if printInformation {
						fmt.Printf("%s\t%s is rotating on %s for next %.2fh\n",
							simulationNow(),
							rt.String(),
							s.String(),
							waitTime)
					}
					// pause goroutine for calculated time
					time.Sleep(time.Duration(waitTime*float64(secondsInHour)) * time.Second)

					// send information to repair train that we did our job
					s.readyToGo <- true
					// wait until repair train leaves
					<-rt.offRail

				case t := <-s.userTrain:
					// when train want's to enter
					// inform last rail it is free
					t.offRail <- true
					// get current time
					currTime := simulationNow()
					writeStat(fmt.Sprintf("%s %s leaves %s\n", currTime, t.String(), t.position.String()))
					// move train to new position (switch)
					t.position = t.route[t.routeIndex]
					writeStat(fmt.Sprintf("%s %s enters %s\n", currTime, t.String(), t.position.String()))
					// calculate time we have to wait
					waitTime := t.position.waitTime(t)
					//if printInformation {
					//	fmt.Printf("%s\t%s is rotating on %s for next %.2fh\n",
					//		currTime,
					//		t.String(),
					//		t.position.String(),
					//		waitTime)
					//}
					// pause goroutine for calculated time
					time.Sleep(time.Duration(waitTime*float64(secondsInHour)) * time.Second)

					// send information to train that we did our job
					s.readyToGo <- true
					// wait until train leaves
					<-t.offRail

					// with probability of 0.5% break (if repair train is ready)
					if 0.005 > rand.Float64() {
						select {
						case *repairRequest <- s:
							// when repair train can accept request
							// wait for information from repair train that train is fixed
							<-s.repaired
							if printInformation {
								fmt.Printf("%s\t%s is repaired\n",
									simulationNow(),
									s.String())
							}
						default:
							// when no repair train can accept request, act like nothing happened
							continue
						}
					}

				case <-s.suspended:
					// when rail is suspended by reservation wait for release or repair train
					select {
					case <-s.released:
						// when reservation is released, continue
						continue
					case rt := <-s.userRepairTrain:
						// when repair train enters, move it along then continue normal work
						rt.offRail <- true
						// calculate time we have to wait
						waitTime := s.waitTime(rt)
						if printInformation {
							fmt.Printf("%s\t%s is rotating on %s for next %.2fh\n",
								simulationNow(),
								rt.String(),
								s.String(),
								waitTime)
						}
						// pause goroutine for calculated time
						time.Sleep(time.Duration(waitTime*float64(secondsInHour)) * time.Second)

						// send information to repair train that we did our job
						s.readyToGo <- true
						// wait until repair train leaves
						<-rt.offRail
					}
				}
			}
		}(switches[i], &toRepairRequest)
	}
	// scan file for platforms, create and save them in railway
	for i := 0; i < p; i++ {
		scanner.Scan()
		line = scanner.Text()
		fields = strings.Fields(line)

		id, _ := strconv.Atoi(fields[0])
		sTime, _ := strconv.Atoi(fields[1])
		from, _ := strconv.Atoi(fields[2])
		to, _ := strconv.Atoi(fields[3])
		repDur, _ := strconv.Atoi(fields[4])

		platform := &Platform{
			id:              id,
			stopTime:        sTime,
			repairDuration:  repDur,
			connects:        []BasicRail{switches[from], switches[to]},
			userTrain:       make(chan *Train),
			userRepairTrain: make(chan *RepairTrain),
			readyToGo:       make(chan bool),
			repaired:        make(chan bool),
			suspended:       make(chan bool),
			released:        make(chan bool),
			reservation:     make(chan bool, 1),
			station:         nil}

		railway[from][to] = append(railway[from][to], platform)
		railway[to][from] = append(railway[to][from], platform)

		// check if station that this platform belongs to exists
		for _, r := range railway[from][to] {
			switch r.(type) {
			case *Platform:
				p := r.(*Platform)
				if p.station != nil {
					// append platform to station
					p.station.platforms = append(p.station.platforms, platform)
					platform.station = p.station
					goto Found
				}
			default:
				continue
			}
		}

		// if no station was found, create one
		platform.station = &Station{
			id:             len(stations),
			platforms:      []*Platform{platform},
			inhabitants:    make([]*Worker, 0),
			operatedTrains: make([]*Train, 0)}
		stations = append(stations, platform.station)

	Found:

		go func(p *Platform, repairRequest *chan interface{}) {
			for {
				select {
				case rt := <-p.userRepairTrain:
					// when repair train want's to enter
					// inform last rail it is free
					rt.offRail <- true
					// calculate time we have to wait
					waitTime := p.waitTime(rt)
					if printInformation {
						fmt.Printf("%s\t%s is on %s for next %.2fh\n",
							simulationNow(),
							rt.String(),
							p.String(),
							waitTime)
					}
					// pause goroutine for calculated time
					time.Sleep(time.Duration(waitTime*float64(secondsInHour)) * time.Second)

					// send information to repair train that we did our job
					p.readyToGo <- true
					// wait until repair train leaves
					<-rt.offRail

				case t := <-p.userTrain:
					// when train want's to enter
					// inform last rail it is free
					t.offRail <- true
					// get current time
					currTime := simulationNow()
					writeStat(fmt.Sprintf("%s %s leaves %s\n", currTime, t.String(), t.position.String()))
					// move train to new position (switch)
					t.position = p
					writeStat(fmt.Sprintf("%s %s enters %s\n", currTime, t.String(), t.position.String()))
					// calculate time we have to wait
					waitTime := t.position.waitTime(t)
					if printInformation {
						fmt.Printf("%s\t%s is on %s for next %.2fh\n",
							currTime,
							t.String(),
							t.position.String(),
							waitTime)
					}

					// array for saving passengers that left
					leaving := make([]*Worker, 0)
					// for every passenger in train
					for _, w := range t.passengers {
						ticket := w.ticket
						// if it's destination
						if ticket.to == p.station {
							// save leaving passenger
							leaving = append(leaving, w)
							if printInformation {
								var format string
								if w.home == p.station {
									format = "%s\t%s leaves %s at %s and returns to home\n"
								} else {
									format = "%s\t%s leaves %s at %s\n"
								}
								fmt.Printf(format,
									simulationNow(),
									w.String(),
									t,
									p.station)
							}
							// leave train
							w.position = p.station
							w.ticket = nil
							w.arrived <- true
						}
					}
					// delete every passenger that left the train from array
					for _, w := range leaving {
						for i, p := range t.passengers {
							if w == p {
								t.passengers = append(t.passengers[:i], t.passengers[i+1:]...)
								break
							}
						}
					}

					// last passenger that got to train index
					var lastPassenger int
					// for every pending passenger on the station
					for i, w := range t.pendingPassengers[p.station] {
						// if there are free places in train
						if t.capacity > len(t.passengers) {
							lastPassenger = i
							if printInformation {
								fmt.Printf("%s\t%s gets on %s at %s\n",
									simulationNow(),
									w.String(),
									t,
									p.station)
							}
							// let passenger in
							t.passengers = append(t.passengers, w)
							w.position = nil
						} else {
							break
						}
					}
					// cut pending passengers array
					if lastPassenger < len(t.pendingPassengers[p.station])-1 {
						t.pendingPassengers[p.station] = t.pendingPassengers[p.station][lastPassenger+1:]
					} else {
						t.pendingPassengers[p.station] = make([]*Worker, 0)
					}

					// pause goroutine for calculated time
					time.Sleep(time.Duration(waitTime*float64(secondsInHour)) * time.Second)

					// send information to train that we did our job
					p.readyToGo <- true
					// wait until train leaves
					<-t.offRail

					// with probability of 1% break (if repair train is ready)
					if 0.01 > rand.Float64() {
						select {
						case *repairRequest <- p:
							// when repair train can accept request
							// wait for information from repair train that train is fixed
							<-p.repaired
							if printInformation {
								fmt.Printf("%s\t%s is repaired\n",
									simulationNow(),
									p.String())
							}
						default:
							// when no repair train can accept request, act like nothing happened
							continue
						}
					}

				case <-p.suspended:
					// when rail is suspended by reservation wait for release or repair train
					select {
					case <-p.released:
						// when reservation is released, continue
						continue
					case rt := <-p.userRepairTrain:
						// when repair train enters, move it along then continue normal work
						rt.offRail <- true
						// calculate time we have to wait
						waitTime := p.waitTime(rt)
						if printInformation {
							fmt.Printf("%s\t%s is on %s for next %.2fh\n",
								simulationNow(),
								rt.String(),
								p.String(),
								waitTime)
						}
						// pause goroutine for calculated time
						time.Sleep(time.Duration(waitTime*float64(secondsInHour)) * time.Second)

						// send information to repair train that we did our job
						p.readyToGo <- true
						// wait until repair train leaves
						<-rt.offRail
					}
				}
			}
		}(platform, &toRepairRequest)
	}

	// scan file for rails, create and save them in railway
	for i := 0; i < r; i++ {
		scanner.Scan()
		line = scanner.Text()
		fields = strings.Fields(line)

		id, _ := strconv.Atoi(fields[0])
		len, _ := strconv.Atoi(fields[1])
		speed, _ := strconv.Atoi(fields[2])
		from, _ := strconv.Atoi(fields[3])
		to, _ := strconv.Atoi(fields[4])
		repDur, _ := strconv.Atoi(fields[5])

		rail := &Rail{
			id:              id,
			len:             len,
			speedLimit:      speed,
			repairDuration:  repDur,
			connects:        []BasicRail{switches[from], switches[to]},
			userTrain:       make(chan *Train),
			userRepairTrain: make(chan *RepairTrain),
			readyToGo:       make(chan bool),
			repaired:        make(chan bool),
			suspended:       make(chan bool),
			released:        make(chan bool),
			reservation:     make(chan bool, 1)}

		railway[from][to] = append(railway[from][to], rail)
		railway[to][from] = append(railway[to][from], rail)

		go func(r *Rail, repairRequest *chan interface{}) {
			for {
				select {
				case rt := <-r.userRepairTrain:
					// when repair train want's to enter
					// inform last rail it is free
					rt.offRail <- true
					// calculate time we have to wait
					waitTime := r.waitTime(rt)
					if printInformation {
						fmt.Printf("%s\t%s is on %s for next %.2fh\n",
							simulationNow(),
							rt.String(),
							r.String(),
							waitTime)
					}
					// pause goroutine for calculated time
					time.Sleep(time.Duration(waitTime*float64(secondsInHour)) * time.Second)

					// send information to repair train that we did our job
					r.readyToGo <- true
					// wait until repair train leaves
					<-rt.offRail

				case t := <-r.userTrain:
					// when train want's to enter
					// inform last rail it is free
					t.offRail <- true
					// move train to new position (switch)
					t.position = r
					// calculate time we have to wait
					waitTime := t.position.waitTime(t)
					//if printInformation {
					//	fmt.Printf("%s\t%s is on %s for next %.2fh\n",
					//		simulationNow(),
					//		t.String(),
					//		t.position.String(),
					//		waitTime)
					//}
					// pause goroutine for calculated time
					time.Sleep(time.Duration(waitTime*float64(secondsInHour)) * time.Second)

					// send information to train that we did our job
					r.readyToGo <- true
					// wait until train leaves
					<-t.offRail

					// with probability of 1% break (if repair train is ready)
					if 0.01 > rand.Float64() {
						select {
						case *repairRequest <- r:
							// when repair train can accept request
							// wait for information from repair train that train is fixed
							<-r.repaired
							if printInformation {
								fmt.Printf("%s\t%s is repaired\n",
									simulationNow(),
									r.String())
							}
						default:
							// when no repair train can accept request, act like nothing happened
							continue
						}
					}

				case <-r.suspended:
					// when rail is suspended by reservation wait for release or repair train
					select {
					case <-r.released:
						// when reservation is released, continue
						continue
					case rt := <-r.userRepairTrain:
						// when repair train enters, move it along then continue normal work
						rt.offRail <- true
						// calculate time we have to wait
						waitTime := r.waitTime(rt)
						if printInformation {
							fmt.Printf("%s\t%s is on %s for next %.2fh\n",
								simulationNow(),
								rt.String(),
								r.String(),
								waitTime)
						}
						// pause goroutine for calculated time
						time.Sleep(time.Duration(waitTime*float64(secondsInHour)) * time.Second)

						// send information to repair train that we did our job
						r.readyToGo <- true
						// wait until repair train leaves
						<-rt.offRail
					}
				}
			}
		}(rail, &toRepairRequest)
	}

	// find every switch connections and save them
	for _, s := range switches {
		for j := range railway[s.id] {
			for _, track := range railway[s.id][j] {
				s.connects = append(s.connects, track)
			}
		}
	}

	// scan file for repair trains, create and save them, then start goroutine for each
	for i := 0; i < rt; i++ {
		scanner.Scan()
		line = scanner.Text()
		fields = strings.Fields(line)

		id, _ := strconv.Atoi(fields[0])
		speed, _ := strconv.Atoi(fields[1])
		start, _ := strconv.Atoi(fields[2])

		repairTrains[i] = &RepairTrain{
			id:       id,
			maxSpeed: speed,
			depot: &Platform{ // create depot platform that is not saved in connections graph
				id:              p,
				stopTime:        30,
				repairDuration:  0,
				connects:        []BasicRail{switches[start]},
				userTrain:       make(chan *Train),
				userRepairTrain: make(chan *RepairTrain),
				readyToGo:       make(chan bool)},
			start:    switches[start],
			offRail:  make(chan bool),
			toRepair: &toRepairRequest}

		go func(p *Platform, repairRequest *chan interface{}) {
			// depot platform only needs to handle repair trains
			for {
				rt := <-p.userRepairTrain
				rt.offRail <- true

				if printInformation {
					fmt.Printf("%s\t%s is in depot %s\n",
						simulationNow(),
						rt.String(),
						p.String())
				}

				p.readyToGo <- true
				<-rt.offRail
			}
		}(repairTrains[i].depot, &toRepairRequest)

		//go repairTrains[i].Run(&switches, &railway)
	}

	// create WaitGroup that will make sure program will not end before trains stop (which they never do)
	waitGroup := new(sync.WaitGroup)
	waitGroup.Add(len(trains))

	// scan file for trains, create and save them, then start goroutine for each
	for i := 0; i < t; i++ {
		scanner.Scan()
		line = scanner.Text()
		fields = strings.Fields(line)

		id, _ := strconv.Atoi(fields[0])
		speed, _ := strconv.Atoi(fields[1])
		capacity, _ := strconv.Atoi(fields[2])
		repDur, _ := strconv.Atoi(fields[3])
		routeLen, _ := strconv.Atoi(fields[4])

		trains[i] = &Train{
			id:                id,
			maxSpeed:          speed,
			capacity:          capacity,
			repairDuration:    repDur,
			route:             make([]*Switch, routeLen),
			stops:             make([]*Station, 0),
			passengers:        make([]*Worker, 0),
			pendingPassengers: make(map[*Station][]*Worker, 0),
			offRail:           make(chan bool),
			repaired:          make(chan bool)}

		scanner.Scan()
		line = scanner.Text()
		fields = strings.Fields(line)

		// read trains route and create it
		for j := 0; j < routeLen; j++ {
			index, _ := strconv.Atoi(fields[j])

			trains[i].route[j] = switches[index]
		}

		lastSwitch := trains[i].route[routeLen-1]
		for j:= 0; j < routeLen; j++ {
			nextSwitch := trains[i].route[j]
		CheckStations:
			for _, station := range stations {
				connects := station.platforms[0].connects
				if (
					connects[0] == lastSwitch &&
					connects[1] == nextSwitch) ||
					(
					connects[1] == lastSwitch &&
					connects[0] == nextSwitch) {
					for _, stop := range trains[i].stops {
						if stop == station {
							break CheckStations
						}
					}
					trains[i].stops = append(trains[i].stops, station)
					station.operatedTrains = append(station.operatedTrains, trains[i])
					trains[i].pendingPassengers[station] = make([]*Worker, 0)
					break CheckStations
				}
			}
			lastSwitch = nextSwitch
		}
	}
	
	// scan file for workers
	for i := 0; i < w; i++ {
		scanner.Scan()
		line = scanner.Text()
		fields = strings.Fields(line)

		id, _ := strconv.Atoi(fields[0])
		platformId, _ := strconv.Atoi(fields[1])

		var home *Station
		// find station with home platform
	Loop:
		for _, s := range stations {
			for _, p := range s.platforms {
				if p.id == platformId {
					home = p.station
					break Loop
				}
			}
		}

		workers[i] = &Worker{
			id:          id,
			home:        home,
			position:    home,
			arrived:     make(chan bool),
			readyToWork: make(chan bool),
			newTask:     make(chan *Task)}
		// add worker to station inhabitants
		home.inhabitants = append(home.inhabitants, workers[i])

		go func(w *Worker) {
			for {
			Wait:
			// wait for task
				w.task = <-w.newTask
				if printInformation {
					fmt.Printf("%s\t%s got task at %s\n",
						simulationNow(),
						w.String(),
						w.task.location)
				}

				if w.task.location == w.home {
					// if task is at home, just do it
					w.doTask()
				} else {
					// look for connection to task location
					departingTrains := w.home.operatedTrains
					arrivingTrains := w.task.location.operatedTrains

					// check if there is direct connection
					for _, d := range departingTrains {
						for _, a := range arrivingTrains {
							if d == a {
								w.getTicket(d, w.task.location)
								w.doTask()
								w.getTicket(a, w.home)
								
								// wait for next task
								goto Wait
							}
						}
					}

					// if no direct connection was found travel with change
					for _, d := range departingTrains {
						for _, sd := range d.stops {
							for _, a := range arrivingTrains {
								for _, sa := range a.stops {
									if sd == sa {
										w.getTicket(d, sd)
										// change at station sd (==sa)
										w.getTicket(a, w.task.location)
										w.doTask()
										w.getTicket(a, sa)
										// change at station sa (==sd)
										w.getTicket(d, w.home)
										
										// wait for next task
										goto Wait
									}
								}
							}
						}
					}
				}
			}
		}(workers[i])
	}

	// print railway stations with their workers and platforms
	if printInformation {
		for _, s := range stations {
			fmt.Printf("%s:\n", s)
			for _, p := range s.platforms {
				fmt.Printf("\t%s\n", p)
			}
			for _, w := range s.inhabitants {
				fmt.Printf("\t%s\n", w)
			}
			for _, t := range s.operatedTrains {
				fmt.Printf("\t%s\n", t)
			}
		}
	}

	// when workers are ready, run trains
	for i := 0; i < t; i++ {
		go trains[i].Run(&switches, &railway, &toRepairRequest, waitGroup)
	}

	// task dispatcher
	go func() {
		for {
			// every 4 hours create new task
			time.Sleep(time.Duration(4*secondsInHour) * time.Second)
			
			taskTime := 30 + rand.Intn(31) // task time is 30-60 minutes
			location := stations[rand.Intn(len(stations))] // random location
			assignedWorkers := make([]*Worker, 0)
			
			// assign from 1 worker to 1/3 of all workers
			for _, i := range rand.Perm(len(workers))[:1+rand.Intn(len(workers)/3)] {
				assignedWorkers = append(assignedWorkers, workers[i])
			}
			
		CheckWorkers:
			for _, w := range assignedWorkers {
				if w.position != w.home || w.task != nil {
					// if any of assigned workers is not available, wait 2 hours
					time.Sleep(time.Duration(2*secondsInHour) * time.Second)
					goto CheckWorkers
				}
			}

			// create task
			task := &Task{
				location: location,
				time:     taskTime,
				workers:  make(map[*Worker]bool, 0),
				arrived:  make(chan *Worker)}

			// run task main function
			go task.Run(assignedWorkers)
		}
	}()

	// if program is in silent mode, run goroutine with informant
	if !printInformation {
		waitGroup.Add(1)
		go informant(waitGroup, trains)
	}

	// wait for all goroutines before ending program
	waitGroup.Wait()
}
