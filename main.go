package main

import (
	"bufio"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Train type represents train object with set id, speed, pasengers capacity,
// route marked by switches, also saves current switch index and position.
type Train struct {
	id, maxSpeed, capacity int
	route                  []*Switch
	routeIndex             int
	position               BasicRail
}

func (t *Train) wait(s float64) {
	time.Sleep(time.Duration(s*float64(secondsInHour)) * time.Second)
}

func (t *Train) String() string {
	return "Train" + strconv.Itoa(t.id)
}

// Run is main Train function, that moves train on it's route
func (t *Train) Run(switches *[]*Switch, railway *[][][]BasicRail, group *sync.WaitGroup) {
	// place train on first Switch in route
	<-t.route[t.routeIndex].lock
	t.position = t.route[t.routeIndex]
	time := simulationNow()
	waitTime := t.position.waitTime(t)
	writeStat(fmt.Sprintf("%s %s enters %s\n", time, t.String(), t.position.String()))
	if printInformation {
		fmt.Printf("%s\t%s is rotating on %s for next %.2fh\n",
			time,
			t.String(),
			t.position.String(),
			waitTime)
	}
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
				// try to move onto rail, that is lock it
				switch r.tryLock() {
				case true:
					// if we got lock
					// unlock last position
					t.position.unlock()
					// get current time
					time := simulationNow()
					writeStat(fmt.Sprintf("%s %s leaves %s\n", time, t.String(), t.position.String()))
					// move train to new position (rail)
					t.position = r
					writeStat(fmt.Sprintf("%s %s enters %s\n", time, t.String(), t.position.String()))
					// calculate time we have to wait
					waitTime := t.position.waitTime(t)
					if printInformation {
						fmt.Printf("%s\t%s is on %s for next %.2fh\n",
							time,
							t.String(),
							t.position.String(),
							waitTime)
					}
					// pause goroutine for calculated time
					t.wait(waitTime)
					// free rail was found, break from loop
					break FindRail
				case false:
					// if rail was occupied try next one
					continue
				}
			}
		}
		// we have to change route index (switch)
		t.routeIndex = (t.routeIndex + 1) % len(t.route)
	LockSwitch:
		// loop until we move onto next switch as it can be occupied
		for {
			// try to move onto switch at routeIndex
			switch t.route[t.routeIndex].tryLock() {
			case true:
				// if we got lock
				// unlock last position
				t.position.unlock()
				// get current time
				time := simulationNow()
				writeStat(fmt.Sprintf("%s %s leaves %s\n", time, t.String(), t.position.String()))
				// move train to new position (switch)
				t.position = t.route[t.routeIndex]
				writeStat(fmt.Sprintf("%s %s enters %s\n", time, t.String(), t.position.String()))
				// calculate time we have to wait
				waitTime := t.position.waitTime(t)
				if printInformation {
					fmt.Printf("%s\t%s is rotating on %s for next %.2fh\n",
						time,
						t.String(),
						t.position.String(),
						waitTime)
				}
				// pause goroutine for calculated time
				t.wait(waitTime)
				// we moved to next switch, break from loop
				break LockSwitch
			case false:
				// if switch was occupied try again
				continue
			}
		}
	}
}

// simulates clock in simulation time
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

// BasicRail is interface for all rail types: Rail, Platform, Switch
type BasicRail interface {
	// waitTime makes sure every rail can return time for given train.
	// Time returned is that the train have to wait before next action.
	waitTime(train *Train) float64
	// String returns string representation of rail
	String() string
	// tryLock tries to lock rail, returns boolean that tells if it was successful
	tryLock() bool
	// unlock unlocks rail for next train
	unlock()
}

// Rail is basic rail for transport
type Rail struct {
	// len - length of rail in km
	// speedLimit - speed limit on rail in km/h
	id, len, speedLimit int
	lock                chan int
}

// Platform is rail on platform or in depot
type Platform struct {
	// stopTime - minimum stop time on platform in minutes
	id, stopTime int
	lock         chan int
}

// Switch connects rails and enables train to move from one to another
type Switch struct {
	// rotationTime - time that is needed to rotate train on Switch in minutes
	id, rotationTime int
	lock             chan int
}

func (r *Rail) waitTime(train *Train) float64 {
	return float64(r.len) / math.Min(float64(r.speedLimit), float64(train.maxSpeed))
}
func (p *Platform) waitTime(train *Train) float64 {
	return float64(p.stopTime) / 60.0
}
func (s *Switch) waitTime(train *Train) float64 {
	return float64(s.rotationTime) / 60.0
}

func (r *Rail) tryLock() bool {
	select {
	case <-r.lock:
		return true
	default:
		return false
	}
}
func (p *Platform) tryLock() bool {
	select {
	case <-p.lock:
		return true
	default:
		return false
	}
}
func (s *Switch) tryLock() bool {
	select {
	case <-s.lock:
		return true
	default:
		return false
	}
}

func (r *Rail) unlock()     { r.lock <- 1 }
func (p *Platform) unlock() { p.lock <- 1 }
func (s *Switch) unlock()   { s.lock <- 1 }

func (r *Rail) String() string     { return "Rail" + strconv.Itoa(r.id) }
func (p *Platform) String() string { return "Platform" + strconv.Itoa(p.id) }
func (s *Switch) String() string   { return "Switch" + strconv.Itoa(s.id) }

func check(e error) {
	if e != nil {
		panic(e)
	}
}

// informant id function for goiroutine that communicates with user
func informant(group *sync.WaitGroup, trains []*Train) {
	defer group.Done()

	s := "`i` for information, `e` to exit"
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Printf("%s: ", s)

		response, err := reader.ReadString('\n')
		check(err)

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

var secondsInHour int
var startTime time.Time
var stat *bufio.Writer
var statMutex sync.Mutex

var printInformation bool
var filename string

func main() {
	startTime = time.Now()

	if len(os.Args) != 3 {
		fmt.Println("need 2 arguments: <input filename [string]> <printInformation [bool]>")
		os.Exit(1)
	} else {
		filename = os.Args[1]
		v, err := strconv.ParseBool(os.Args[2])
		printInformation = v
		check(err)
	}

	// create file for statistics, create writer
	f, err := os.Create("stats")
	check(err)
	stat = bufio.NewWriter(f)

	// open file in railway data, create scanner
	data, err := os.Open(filename)
	check(err)
	scanner := bufio.NewScanner(data)
	scanner.Scan()
	line := scanner.Text()
	fields := strings.Fields(line)

	// s - number of Switches defined
	s, err := strconv.Atoi(fields[0])
	// p - number of Platforms defined
	p, err := strconv.Atoi(fields[1])
	// r - number of Rails defined
	r, err := strconv.Atoi(fields[2])
	// t - number of Trains defined
	t, err := strconv.Atoi(fields[3])
	// hour - number of seconds for hour simulation
	hour, err := strconv.Atoi(fields[4])

	check(err)

	secondsInHour = hour

	// create arrays of pointers to switches and trains
	switches := make([]*Switch, s)
	trains := make([]*Train, t)

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

		id, err := strconv.Atoi(fields[0])
		time, err := strconv.Atoi(fields[1])

		check(err)

		switches[i] = &Switch{id: id, rotationTime: time, lock: make(chan int, 1)}
		switches[i].lock <- 1
	}
	// scan file for platforms, create and save them in railway
	for i := 0; i < p; i++ {
		scanner.Scan()
		line = scanner.Text()
		fields = strings.Fields(line)

		id, err := strconv.Atoi(fields[0])
		time, err := strconv.Atoi(fields[1])
		from, err := strconv.Atoi(fields[2])
		to, err := strconv.Atoi(fields[3])

		check(err)

		platform := &Platform{id: id, stopTime: time, lock: make(chan int, 1)}
		platform.lock <- 1

		railway[from][to] = append(railway[from][to], platform)
		railway[to][from] = append(railway[to][from], platform)
	}

	// scan file for rails, create and save them in railway
	for i := 0; i < r; i++ {
		scanner.Scan()
		line = scanner.Text()
		fields = strings.Fields(line)

		id, err := strconv.Atoi(fields[0])
		len, err := strconv.Atoi(fields[1])
		speed, err := strconv.Atoi(fields[2])
		from, err := strconv.Atoi(fields[3])
		to, err := strconv.Atoi(fields[4])

		check(err)

		rail := &Rail{id: id, len: len, speedLimit: speed, lock: make(chan int, 1)}
		rail.lock <- 1

		railway[from][to] = append(railway[from][to], rail)
		railway[to][from] = append(railway[to][from], rail)
	}

	// set logger to append time to logs
	log.SetFlags(log.Ltime)

	// create WaitGroup that will make sure program will not end before trains stop (which they never do)
	waitGroup := new(sync.WaitGroup)
	waitGroup.Add(len(trains))

	// scan file for trains, create and save them, then start goroutine for each
	for i := 0; i < t; i++ {
		scanner.Scan()
		line = scanner.Text()
		fields = strings.Fields(line)

		id, err := strconv.Atoi(fields[0])
		speed, err := strconv.Atoi(fields[1])
		capacity, err := strconv.Atoi(fields[2])
		routeLen, err := strconv.Atoi(fields[3])

		check(err)

		trains[i] = &Train{id, speed, capacity, []*Switch{}, 0, nil}

		scanner.Scan()
		line = scanner.Text()
		fields = strings.Fields(line)

		// read trains route and create it
		for j := 0; j < routeLen; j++ {
			index, err := strconv.Atoi(fields[j])

			check(err)

			trains[i].route = append(trains[i].route, switches[index])
		}

		go trains[i].Run(&switches, &railway, waitGroup)
	}

	// if program is in silent mode, run goroutine with informant
	if !printInformation {
		waitGroup.Add(1)
		go informant(waitGroup, trains)
	}

	// wait for all goroutines before ending program
	waitGroup.Wait()
}
