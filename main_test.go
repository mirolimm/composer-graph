package main

import (
	"bytes"
	"fmt"
	"testing"
)

var txt = []byte(`
services:

  srv1:
    image: vault:0.7.0
    build:
      context: ctx1
    environment:
      E1: "env1"
    ports:
      - "21:21"
      - "11:11"
    depends_on:
      - redis
    networks:
      - n1

  # comment1
  # comment2
  srv2:
    image: postgres:9.5
    depends_on:
      - redis
      #- mongo
      - srv1
    volumes:
      - ./callstats.js:/code
    working_dir: /code
    build:
      context: ctx2
    ports:
      - "31:21"
      - "41:11"
    networks:
      - n2
    command: "npm start"
    environment:
      E1: "env2"

  srv3:
    build:
      context: coturn-docker
    environment:
      SHAREDSECRET: "test" # for short term credentials, known to auth service
      EXTERNAL_IP: coturn
      MAX_BPS: "1000000"
      USER_QUOTA: "20"
      TOTAL_QUOTA: "100"
      REDIS_STATSDB: "ip=redis passwd=test"
      VERBOSE: "true"
    ports:
      - "31000:3478"
      - "31000:3478/udp"
    networks:
      - callstats
    depends_on:
      - redis

  #########################
  # the srv4
  #########################

  srv4:
    build:
      context: webrtc-demoapp
    environment:
      SSL: "true"
      port: 8081
      portSSL: 4440
      # one needs to get the APPID and APPSECRET from dashboard
      APPID: "662712506"
      APPSECRET: "JusbctbKCw3l:hxO3ow4ybnArse14pgSPC5"
      JWT: "false"
      TARGET: "local"
    ports:
      - "4440:4440"
    networks:
      - callstats
    depends_on:
      - srv3
      - srv2
    command: "npm start"

volumes:
  dashboard_creds:
  access_management_creds:

networks:
  callstats: {}

`)

func TestParser(t *testing.T) {
	fmt.Println("text size", len(txt))
	m := parser(txt)
	fmt.Println("parser result", m)
	if len(m) != 4 {
		t.Errorf("expected %d got %d", 4, len(m))
	}

	for k, service := range m {
		fmt.Printf("key:>>%s<<image>>%s<<\n", k, service.Image)
		for k2, v2 := range service.DependsOn {
			fmt.Printf("key:%d, val:>>%s<<\n", k2, v2)
		}
	}

	if len(m["srv2"].DependsOn) != 2 {
		t.Errorf("expected number of srv1 deps %d, got %d", 2, len(m["srv2"].DependsOn))
	}
	img := m["srv2"].Image
	if img != "postgres:9.5" {
		t.Errorf("expected img %s, got %s", "postgres:9.5", img)
	}

	srv3 := m["srv3"]
	if srv3.Image != "" && len(srv3.DependsOn) == 1 {
		t.Errorf("expected img %v, got %v", "", srv3)
	}

	srv4 := m["srv4"]
	if srv4.Image != "" && len(srv4.DependsOn) == 2 {
		t.Errorf("expected img %v, got %v", "", srv4)
	}

}

func TestKMPsearch(t *testing.T) {
	str := []byte("depends_on")

	expected := bytes.Index(txt, str)
	got := search(str, txt)
	if got == -1 {
		t.Error("could not find")
		return
	}

	if expected != got {
		t.Errorf("expected %d %s, got %d %s", expected, string(txt[expected]), got, string(txt[got]))
	}
}

var input = "sample1.yml"
