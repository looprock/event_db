#!/bin/bash
# Endpoint: GET /api/events/by-date?date=YYYY-MM-DD
http GET http://k3s-ingress:8081/api/events tag==$1
# http GET http://localhost:8081/api/events tag==$1
# http GET http://localhost:8081/api/events/by-date date==$1
# http GET http://k3s-ingress:8081/api/events/by-date date==$1
