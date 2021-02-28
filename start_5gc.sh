#!/usr/bin/env bash

# clear history data in mongodb
echo "db.dropDatabase()" | mongo free5gc

# install gtpu module
cd $HOME/go/pkg/mod/github.com/PrinzOwO/gtp5g && make install
cd $HOME/free5gc

# Check OS
if [ -f /etc/os-release ]; then
    # freedesktop.org and systemd
    . /etc/os-release
    OS=$NAME
    VER=$VERSION_ID
else
    # Fall back to uname, e.g. "Linux <version>", also works for BSD, etc.
    OS=$(uname -s)
    VER=$(uname -r)
    echo "This Linux version is too old: $OS:$VER, we don't support!"
    exit 1
fi

# configure dump flag
while getopts 'o' OPT;
do
    case $OPT in
        o) DUMP_NS=True;;
    esac
done
shift $(($OPTIND - 1))

# configure GO environment variable
GOPATH=$HOME/go
if [ $OS == "Ubuntu" ]; then
    GOROOT=/usr/local/go
elif [ $OS == "Fedora" ]; then
    GOROOT=/usr/lib/golang
fi
PATH=$PATH:$GOPATH/bin:$GOROOT/bin

# configure environment for UPF
UPFNS="UPFns"
EXEC_UPFNS="sudo -E ip netns exec ${UPFNS}"

export GIN_MODE=release

# Setup network namespace
sudo ip netns add ${UPFNS}

sudo ip link add veth0 type veth peer name veth1
sudo ip link set veth0 up
sudo ip addr add 60.60.0.1 dev lo
sudo ip addr add 10.200.200.1/24 dev veth0
sudo ip addr add 10.200.200.2/24 dev veth0

sudo ip link set veth1 netns ${UPFNS}

${EXEC_UPFNS} ip link set lo up
${EXEC_UPFNS} ip link set veth1 up
${EXEC_UPFNS} ip addr add 60.60.0.101 dev lo
${EXEC_UPFNS} ip addr add 10.200.200.101/24 dev veth1
${EXEC_UPFNS} ip addr add 10.200.200.102/24 dev veth1

if [ ${DUMP_NS} ]
then
    ${EXEC_UPFNS} tcpdump -i any -w ${UPFNS}.pcap &
    TCPDUMP_PID=$(sudo ip netns pids ${UPFNS})
    sudo -E tcpdump -i lo -w default_ns.pcap &
    LOCALDUMP=$!
fi

# start upf
cd $HOME/free5gc/src/upf/build && ${EXEC_UPFNS} ./bin/free5gc-upfd -f config/upfcfg.test.yaml &
sleep 5

# start others NF
PID_LIST=()
NF_LIST="nrf amf smf udr pcf udm nssf ausf"
for NF in ${NF_LIST}; do
    $HOME/free5gc/bin/${NF} &
    PID_LIST+=($!)
    sleep 2
done

# start webUI
go run $HOME/free5gc/webconsole/server.go &

# bind mango to cpu 1
taskset -pc 1 `pgrep mongodb`
# bind nrf udr pcf udm nssf ausf to cpu 2
taskset -apc 2 `pgrep nrf`
taskset -apc 2 `pgrep udr`
taskset -apc 2 `pgrep pcf`
taskset -apc 2 `pgrep udm`
taskset -apc 2 `pgrep nssf`
taskset -apc 2 `pgrep ausf`
# bind amf,smf,upf to cpu 3,4,5
taskset -apc 3 `pgrep amf`
taskset -apc 4 `pgrep smf`
taskset -apc 5 `pgrep free5gc-upfd`

echo "All 5GC NFs are started, you may start procedure testing ..."

function terminate()
{
    # kill amf first
    while $(sudo kill -SIGINT ${PID_LIST[2]} 2>/dev/null); do
        sleep 2
    done

    for ((idx=${#PID_LIST[@]}-1;idx>=0;idx--)); do
        sudo kill -SIGKILL ${PID_LIST[$idx]}
    done
    sleep 2

    # kill upf
    sudo killall -15 free5gc-upfd
    sleep 2

    if [ ${DUMP_NS} ]
    then
        ${EXEC_UPFNS} kill -SIGINT ${TCPDUMP_PID}
        sudo -E kill -SIGINT ${LOCALDUMP}
    fi

    cd $HOME/free5gc
    mkdir -p testkeylog
    for KEYLOG in $(ls *sslkey.log); do
        mv $KEYLOG testkeylog
    done

    sudo ip link del veth0
    sudo ip netns del ${UPFNS}
    sudo ip addr del 60.60.0.1/32 dev lo

    sleep 2
}

trap terminate SIGINT
wait ${PID_LIST}

