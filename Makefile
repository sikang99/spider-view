#
# Makefile for spider-view
#
PROG=spider-view
VERSION=0.0.0
BUILD=$(VERSION).2
#LOCAL_URL="wss://localhost:8267/live/ws/pub?channel=bq5ame6g10l3jia3h0ng"
#REMOTE_URL="wss://cobot.center:8267/live/ws/pub?channel=bq5ame6g10l3jia3h0ng"
LOCAL="localhost:8267"
REMOTE="cobot.center:8267"
#----------------------------------------------------------------------------------
usage:
	@echo "usage: make [build|run] for $(PROG)"
#----------------------------------------------------------------------------------
build b:
	go build -mod=mod -o $(PROG) *.go
build-mac bm:
	GOOS=darwin GOARCH=arm64 go build -mod=mod -o $(PROG).linux *.go
build-linux bl:
	GOOS=linux GOARCH=amd64 go build -mod=mod -o $(PROG).linux *.go
build-windows bw:
	GOOS=windows GOARCH=amd64 go build -mod=mod -o $(PROG).windows *.go
build-name bn:
	mv spider-device bin/spider-device-$(BUILD)-darwin-amd64
#----------------------------------------------------------------------------------
run r:
	@echo "> make (run) [help|local|remote]"
rh:
	./$(PROG) -h
run-local rl:
	@echo "> make (run-local) [c1|c2|s1|s2]"
rlc:
	./$(PROG)
rlc1:
	./$(PROG) -spider=$(LOCAL) -brate=2000000 -kint=30
rlc2:
	./$(PROG) -spider=$(LOCAL) -vlabel=Iriun
rls:
	./$(PROG) -vtype=screen
rls1:
	./$(PROG) -spider=$(LOCAL) -vtype=screen
rls2:
	./$(PROG) -spider=$(LOCAL) -vtype=screen -vlabel=1
run-remote rr:
	@echo "> make (run-remote) [c1|c2|s1|s2]"
rrc:
	./$(PROG) -spider=$(REMOTE)
rrc1:
	./$(PROG) -spider=$(REMOTE) -brate=700000
rrc2:
	./$(PROG) -spider=$(REMOTE) -vlabel=Iriun
rrs1:
	./$(PROG) -spider=$(REMOTE) -vtype=screen
rrs2:
	./$(PROG) -spider=$(REMOTE) -vtype=screen -vlabel=1

# ./spider-device -spider="cobot.center:8267" -ice="cobot.center:3478" -vtype=screen 
#----------------------------------------------------------------------------------
clean:
	rm -f $(PROG)*

build-run br::
	@make build
	@make rl

#----------------------------------------------------------------------------------
MSG="enhancing"
git g:
	@echo "> make (git:g) [update|store]"

git-update gu:
	git add .
	git commit -a -m "$(BUILD),$(USER) - $(MSG)"
	git push

git-store gs:
	git config credential.helper store
#----------------------------------------------------------------------------------

