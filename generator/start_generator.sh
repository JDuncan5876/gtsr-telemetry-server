#!/bin/bash
docker exec $(docker ps | grep telemetry-server_server | awk '{print $NF}') go run generator/data_generator.go &