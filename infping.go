// infping.go copyright Tor Hveem
// License: MIT

package main

import (
	"bufio"
	"fmt"
	client "github.com/influxdata/influxdb/client/v2"
	"github.com/pelletier/go-toml"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func herr(err error) {
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func perr(err error) {
	if err != nil {
		fmt.Println(err)
	}
}

func slashSplitter(c rune) bool {
	return c == '/'
}

func readPoints(config *toml.TomlTree, con client.Client) {
	args := []string{"-B 1", "-D", "-r0", "-O 0", "-Q 10", "-p 1000", "-l"}
	hosts := config.Get("hosts.hosts").([]interface{})
	for _, v := range hosts {
		host, _ := v.(string)
		args = append(args, host)
	}
	log.Printf("Going to ping the following hosts: %q", hosts)
	cmd := exec.Command("/usr/bin/fping", args...)
	stdout, err := cmd.StdoutPipe()
	herr(err)
	stderr, err := cmd.StderrPipe()
	herr(err)
	cmd.Start()
	perr(err)

	buff := bufio.NewScanner(stderr)
	for buff.Scan() {
		text := buff.Text()
		fields := strings.Fields(text)
		// Ignore timestamp
		if len(fields) > 1 {
			host := fields[0]
			data := fields[4]
			dataSplitted := strings.FieldsFunc(data, slashSplitter)
			// Remove ,
			dataSplitted[2] = strings.TrimRight(dataSplitted[2], "%,")
			sent, recv, lossp := dataSplitted[0], dataSplitted[1], dataSplitted[2]
			min, max, avg := "", "", ""
			// Ping times
			if len(fields) > 5 {
				times := fields[7]
				td := strings.FieldsFunc(times, slashSplitter)
				min, avg, max = td[0], td[1], td[2]
			}
			//log.Printf("Host:%s, loss: %s, min: %s, avg: %s, max: %s", host, lossp, min, avg, max)
			writePoints(config, con, host, sent, recv, lossp, min, avg, max)
		}
	}
	std := bufio.NewReader(stdout)
	line, err := std.ReadString('\n')
	perr(err)
	log.Printf("stdout:%s", line)
}

func writePoints(config *toml.TomlTree, clnt client.Client, host string, sent string, recv string, lossp string, min string, avg string, max string) {
	db := config.Get("influxdb.db").(string)
	loss, _ := strconv.Atoi(lossp)
	tags := map[string]string{"host": host}
	fields := map[string]interface{}{}

	if min != "" && avg != "" && max != "" {
		min, _ := strconv.ParseFloat(min, 64)
		avg, _ := strconv.ParseFloat(avg, 64)
		max, _ := strconv.ParseFloat(max, 64)
		fields = map[string]interface{}{
			"loss": loss,
			"min":  min,
			"avg":  avg,
			"max":  max,
		}
	} else {
		fields = map[string]interface{}{
			"loss": loss,
		}
	}

	bp, _ := client.NewBatchPoints(client.BatchPointsConfig{
		Database:  db,
		Precision: "s",
	})

	pt, err := client.NewPoint("ping", tags, fields, time.Now())
	if err != nil {
		log.Fatal("Error: ", err)
	}

	bp.AddPoint(pt)

	err = clnt.Write(bp)
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	config, err := toml.LoadFile("/etc/config.toml")
	if err != nil {
		fmt.Println("Error:", err.Error())
		os.Exit(1)
	}

	host := config.Get("influxdb.host").(string)
	port := config.Get("influxdb.port").(string)
	username := config.Get("influxdb.user").(string)
	password := config.Get("influxdb.pass").(string)

	c, err := client.NewHTTPClient(client.HTTPConfig{
		Addr:     fmt.Sprintf("http://%s:%s", host, port),
		Username: username,
		Password: password,
	})

	if err != nil {
		log.Fatalln("Error: ", err)
	}

	// Add a 60 second timeout to connect
	dur, ver, err := c.Ping(60)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Connected to influxdb - %s, %v", ver, dur)

	readPoints(config, c)
}
