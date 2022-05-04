#!/bin/bash

# @Author: Jonathan Hirsch
# @Date:   2022-04-16
# Purpose: initialize required apt packages in specified ubuntu VMs, upload code to vms, run and control servers

RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'

shopt -s extglob nullglob

user="azureuser"
ips=("52.146.9.129" "20.115.41.241" "20.115.46.30" "20.115.41.200" "20.115.42.189" "20.106.216.88" "20.106.216.50" "20.83.182.178" "20.106.216.105" "20.231.76.135" "20.106.216.100")
clientip="20.115.41.254"
tracingip="20.115.43.176"
pass="fwCm^m6XR41s1v&m"

uploadToServer() {
  sshpass -p "$pass" scp "project.zip" "$user@$1:~/project.zip"
  echo -e "Uploaded project.zip to $2"
  sshpass -p "$pass" ssh "$user@$1" 'rm -rf project; unzip -q project.zip -d project; rm project.zip'
  echo -e "${GREEN}Unzipped project.zip in server $2 to ~/project and removed zip${NC}"
}

uploadFiles() {
  ## zip but ignore files ending in .log
  zip -r -q "project.zip" "$1" -x "*.log"
  echo -e "${GREEN}done making zip ${NC}"
  for ip in "${!ips[@]}"; do
    echo "Uploading project to server $((ip + 1)) with ip ${ips[$ip]}"
    uploadToServer "${ips[$ip]}" "$((ip + 1))" &
    sleep 0.2
  done

  echo "Uploading project to tracing sever with ip $tracingip"
  uploadToServer "$tracingip" "tracing" &

  echo "Uploading project to client sever with ip $clientip"
  uploadToServer "$clientip" "client" &

  wait
  echo -e "${GREEN} √√√ Done uploading files ${NC}"

  rm project.zip
}

startClient() {
  echo -e "Starting client with ip $clientip"
  sshpass -p "$pass" ssh "$user@$clientip" 'cd ~/project/; go run ./cloud/client/main.go'
  echo -e "${GREEN} √√√ Client Exited${NC}"
}

killClient() {
  echo -e "Killing client with ip $clientip"
  sshpass -p "$pass" ssh "$user@$clientip" "lsof -i -Fp | head -n 1 | sed 's/^p//' | xargs -r kill"
  echo -e "${GREEN} √√√ Client killed${NC}"
}

killTrace() {
  echo -e "Killing tracing server with ip $tracingip"
  sshpass -p "$pass" ssh "$user@$tracingip" "lsof -i -Fp | head -n 1 | sed 's/^p//' | xargs -r kill"
  echo -e "${GREEN} √√√ Tracing server killed${NC}"
}

trace() {
  echo "starting tracing server.."
  sshpass -p "$pass" ssh "$user@$tracingip" 'cd project; go run ./cloud/tracer/main.go'
}

getTrace() {
  echo "getting trace.."
  sshpass -p "$pass" scp "$user@$tracingip:~/project/*_output.log" "./"
  echo -e "${GREEN} √√√ Done getting trace ${NC}"
}

startOneServer() {
  num="$(($1 - 1))"
  sshpass -p "$pass" ssh "$user@${ips[$num]}" "cd project; go run ./cloud/server$1/main.go"
}

startAll() {
  echo "Starting tracing server..."
  trace &
  # sleep 2 seconds to make sure the tracing server is up
  sleep 2
  echo "Starting servers..."
  for i in "${!ips[@]}"; do
    startOneServer "$((i + 1))" &
    sleep 0.2
  done
  wait
  echo "All Servers killed"
}

setup() {
  echo "setting up server $1"
  if [ "$1" = "t" ]; then
    echo "setting up tracing server"
    sshpass -p "$pass" ssh -oStrictHostKeyChecking=no "$user@$tracingip" 'sudo apt update && sudo apt upgrade -y && sudo apt install golang make unzip -y'
    echo -e "${GREEN} √√√ Done setting up tracing server${NC}"
  elif [ "$1" = "c" ]; then
    echo "setting up client"
    sshpass -p "$pass" ssh -oStrictHostKeyChecking=no "$user@$clientip" 'sudo apt update && sudo apt upgrade -y && sudo apt install golang make unzip -y'
    echo -e "${GREEN} √√√ Done setting up cloud client ${NC}"
  else
    num="$(($1 - 1))"
    sshpass -p "$pass" ssh -oStrictHostKeyChecking=no "$user@${ips[$num]}" 'sudo apt update && sudo apt upgrade -y && sudo apt install golang make unzip -y'
    echo -e "${GREEN} √√√ Done setting up server #$1${NC}"
  fi
}

killAll() {
  for ip in "${!ips[@]}"; do
    echo "Killing all paxosFD processes on server $((ip + 1)) with ip ${ips[$ip]}"
    sshpass -p "$pass" ssh "$user@${ips[$ip]}" "lsof -i -Fp | head -n 1 | sed 's/^p//' | xargs -r kill" && echo "Server $((ip + 1)) killed" &
    sleep 0.2
  done

  echo "Killing all paxosFD processes on client server server with ip $clientip"
  sshpass -p "$pass" ssh "$user@$clientip" "lsof -i -Fp | head -n 1 | sed 's/^p//' | xargs -r kill"
  echo "Client server killed"

  echo "Killing all paxosFD processes on tracing server with ip $tracingip"
  sshpass -p "$pass" ssh "$user@$tracingip" "lsof -i -Fp | head -n 1 | sed 's/^p//' | xargs -r kill"
  echo "Tracing server killed"

  wait
  echo -e "${GREEN} √√√ All Servers killed √√√ ${NC}"
}

killOneServer() {
  num="$(($1 - 1))"
  echo "Killing all paxosFD processes on server $1 with ip ${ips[$num]}"
  sshpass -p "$pass" ssh "$user@${ips[$num]}" "lsof -i -Fp | head -n 1 | sed 's/^p//' | xargs -r kill"
  echo ️"💀💀💀 Server $1 killed 💀💀💀️️"
}

partition() {
  num="$(($1 - 1))"
  other="$(($2 - 1))"
  echo "Partitioning: server $1 and server $2 can no longer communicate"
  sshpass -p "$pass" ssh "$user@${ips[$num]}" "sudo iptables -A INPUT -s ${ips[$other]} -j DROP"
  sshpass -p "$pass" ssh "$user@${ips[$other]}" "sudo iptables -A INPUT -s ${ips[$num]} -j DROP"
  echo "💔💔💔 Done partitioning servers $1 and $2 💔💔💔"
}

unpartition() {
  num="$(($1 - 1))"
  other="$(($2 - 1))"
  echo "Unpartitioning: server $1 and server $2 can now communicate"
  sshpass -p "$pass" ssh "$user@${ips[$num]}" "sudo iptables -D INPUT -s ${ips[$other]} -j DROP"
  sshpass -p "$pass" ssh "$user@${ips[$other]}" "sudo iptables -D INPUT -s ${ips[$num]} -j DROP"
  echo "🌐🌐🌐 Done unpartitioning servers $1 and $2 🌐🌐🌐"
}

unpartitionAll() {
  for server in "${!ips[@]}"; do
    echo "Resetting IP Tables: server $((server + 1))"

    sshpass -p "$pass" ssh "$user@${ips[server]}" "sudo iptables -P INPUT ACCEPT && sudo iptables -P FORWARD ACCEPT &&
 sudo iptables -P OUTPUT ACCEPT && sudo iptables -t nat -F && sudo iptables -t mangle -F && sudo iptables -F && sudo iptables -X" &
    sleep 0.2
  done
  wait
  echo -e "${GREEN}All servers can communicate again${NC}"
}

ssh() {
  ## check if $1 is 'c' or 't'
  if [ "$1" = "c" ]; then
    echo "Connecting to client server"
    sshpass -p "$pass" ssh "$user@$clientip"
  elif [ "$1" = "t" ]; then
    echo "Connecting to tracing server"
    sshpass -p "$pass" ssh "$user@$tracingip"
  else
    num="$(($1 - 1))"
    echo "Connecting to server $1 with ip ${ips[$num]}"
    sshpass -p "$pass" ssh "$user@${ips[$num]}"
  fi
}

partitionMultipleServers(){
  inputarray=$1
  for i in "${inputarray[@]:2}"; do
            partition "$1" "$i" &
            sleep 0.2
          done
          wait
          echo -e "${GREEN}Server $1 partitioned from:" "${inputarray[@]:2}" "${NC}"
}

unpartitionMultipleServers(){
  inputarray=$1
for i in "${inputarray[@]:2}"; do
            unpartition "$1" "$i" &
            sleep 0.2
          done
          wait
          echo -e "${GREEN}Server $1 unpartitioned from:" "${inputarray[@]:2}" "${NC}"
}

startMultipleServers() {
  echo -e "${GREEN}Servers started:" "${@}" "${NC}"
  for i in "$@"; do
    startOneServer "$i" &
    sleep 0.2
  done
  wait
}

setupMultipleServers() {
  for i in "$@"; do
    setup "$i" &
    sleep 0.2
  done
  wait
  echo -e "${GREEN}Servers Setup:" "${@}" "${NC}"
}

killMultipleServers() {
  for i in "$@"; do
    killOneServer "$i" &
    sleep 0.2
  done
  wait
  echo -e "${GREEN}Servers killed:" "${@}" "${NC}"
}

showPorts(){
  torun="while :; do lsof -i | grep LISTEN; echo '============='; sleep 5; done"
  if [ "$1" = "c" ]; then
      echo "Displaying used lsof ports in cloud client..."
      echo " will run '$torun' until canceled"
      sleep 5
      sshpass -p "$pass" ssh "$user@$clientip" "$torun"
    elif [ "$1" = "t" ]; then
      echo "Displaying used lsof ports in tracing server"
      echo " will run $torun until canceled"
      sleep 5
      sshpass -p "$pass" ssh "$user@$tracingip" "$torun"
    else
      num="$(($1 - 1))"
      echo "Displaying used lsof ports in server $1 with ip ${ips[$num]}"
      echo " will run $torun until canceled"
      sleep 5
      sshpass -p "$pass" ssh "$user@${ips[$num]}" "$torun"
    fi

}

interactive() {
  echo "Starting interactive mode"
  echo "Type 'help' at any time for a list of commands"
  while true; do
    read -p "> " input
    inputarray=($input)
    case ${inputarray[0]} in
    help | h)
      echo "Commands:"
      echo "  separate server numbers in <[servers]> with a space"
      echo "   "
      echo "  help | h - prints this help message"
      echo "  setup | i <[servers]> - sets up specified servers"
      echo "  showPorts | sp <[server]> - shows ports used by specified server, 'c' for cloud client, 't' for tracing server. Runs until canceled"
      echo "  ssh <server> - ssh into server, 'c' for client, 't' for tracing server"
      echo "  upload | u <path> - uploads folder at <path> to all servers"
      echo "  exit | q - exits interactive mode"
      echo "          "
      echo "  client | c - start the cloud client"
      echo "  start | s - <[servers]> starts specified servers"
      echo "  startAll | sa - starts all servers"
      echo "  startAndTrace | st <[servers]> - starts the tracing server and specified servers"
      echo "  trace | t - starts tracing on server"
      echo "  gettrace | gt - copies the traces from the tracing server to the current directory"
      echo "   "
      echo "  kill | k <[servers]> - kills specified servers"
      echo "  killall | ka - kills all servers"
      echo "  killClient | kc - kill the cloud client"
      echo "  killTrace | kt - kills tracing server"
      echo "   "
      echo "  partition | p <server1> <[servers]> - partitions server <server1> from server <[servers]>"
      echo "  unpartition | up <server1> <[servers]> - unpartitions server <server1> from <[servers]>"
      echo "  unpartitionall | upa - unpartitions all servers by resetting their IPTables"

      ;;
    startAll | sa)
      if [ ${#inputarray[@]} -gt 1 ]; then
        echo -e "${RED}Error -- StartAll: too many arguments ${NC} "
      else
        startAll &
      fi
      ;;
    start | s)
      # show error if length is less than 2
      if [ ${#inputarray[@]} -lt 2 ]; then
        echo -e "${RED}Error -- Start: too few arguments ${NC} "
      else
       startMultipleServers "${inputarray[@]:1}" &
      fi
      ;;
    startAndTrace | st)
      if [ ${#inputarray[@]} -lt 2 ]; then
        echo -e "${RED}Error -- Start: too few arguments ${NC} "
      else
        trace &
        sleep 2
        startMultipleServers "${inputarray[@]:1}" &
      fi
      ;;
    client | c)
      if [ ${#inputarray[@]} -gt 1 ]; then
        echo -e "${RED}Error -- Client: too many arguments ${NC} "
      else
        startClient &
      fi
      ;;
    killClient | kc)
      if [ ${#inputarray[@]} -gt 1 ]; then
        echo -e "${RED}Error -- KillClient: too many arguments ${NC} "
      else
        killClient &
      fi
      ;;
    killall | ka)
      if [ ${#inputarray[@]} -gt 1 ]; then
        echo -e "${RED} Error -- KillAll: too many arguments ${NC}"
      else
        killAll &
      fi
      ;;
    kill | k)
      if [ ${#inputarray[@]} -lt 2 ]; then
        echo -e "${RED} Error -- Kill: too few arguments ${NC}"
      else
        killMultipleServers "${inputarray[@]:1}" &
      fi
      ;;
    setup | i)
      # show error if length is less than 2
      if [ ${#inputarray[@]} -lt 2 ]; then
        echo -e "${RED} Error -- Setup: too few arguments ${NC}"
      else
        setupMultipleServers "${inputarray[@]:1}" &
      fi
      ;;
    partition | p)
      # show error if length is less than 3
      if [ ${#inputarray[@]} -lt 3 ]; then
        echo -e "${RED} Error -- Partition: too few arguments ${NC}"
      else
        partitionMultipleServers "${inputarray[1]}" "${inputarray[@]:2}" &
      fi
      ;;
    unpartition | up)
      # show error if length is less than 3
      if [ ${#inputarray[@]} -lt 3 ]; then
        echo -e "${RED} Error -- Unpartition: too few arguments ${NC}"
      else
        unpartitionMultipleServers "${inputarray[@]:1}" &
      fi
      ;;
    unpartitionall | upa)
      unpartitionAll &
      ;;
    trace | t)
      if [ ${#inputarray[@]} -gt 1 ]; then
        echo -e "${RED} Error -- Trace: too many arguments ${NC}"
      else
        trace &
      fi
      ;;
    killTrace | kt)
      if [ ${#inputarray[@]} -gt 1 ]; then
        echo -e "${RED} Error -- KillTrace: too many arguments ${NC}"
      else
        killTrace &
      fi
      ;;
    upload | u)
      # show error if length is not 2
      if [ ${#inputarray[@]} -lt 2 ]; then
        echo -e "${RED} Error -- Upload: too few arguments ${NC}"
      else
        if [ ! -d "${inputarray[1]}" ]; then
          echo -e "${RED} Error -- Upload: path does not exist ${NC}"
        else
          uploadFiles "${inputarray[1]}" &
        fi
      fi
      ;;
    gettrace | gt)
      if [ ${#inputarray[@]} -gt 1 ]; then
        echo -e "${RED} Error -- GetTrace: too many arguments ${NC}"
      else
        getTrace &
      fi
      ;;
    ssh)
      if [ ${#inputarray[@]} -lt 2 ]; then
        echo -e "${RED} Error -- SSH: too few arguments ${NC}"
      else
        ssh "${inputarray[1]}"
      fi
      ;;
    showPorts | sp)
      if [ ${#inputarray[@]} -lt 2 ]; then
              echo -e "${RED} Error -- ShowServices: too few arguments ${NC}"
            else
              showPorts "${inputarray[1]}"
            fi
      ;;
    exit | e | q)
      break
      ;;
    *)
      echo "Unknown command: '${inputarray[0]}'. Type 'help' for usage information."
      ;;
    esac
  done

  exit 0
}

usage() {
  echo -e "$Usage: "
  echo -e "  ${0} [option] <args>"
  echo -e "Options:"
  echo -e "  -h, --help:"
  echo -e "     Print this help message"
  echo -e "  -i, --setup <n>:"
  echo -e "     Setup the server #n, or 't' for tracing, 'c' for client"
  echo -e "  -r, --run <n>:"
  echo -e "     Start interactive mode"
  echo -e "  --ssh <n>:"
  echo -e "     ssh into server #n or 't' for tracing, 'c' for client"
  echo -e "  -u, --upload <path>:"
  echo -e "     Zip & upload all files and folders at given path to all servers"
  echo -e "                        "
  echo -e "  -c, --client:"
  echo -e "     Start the cloud client"
  echo -e "  -s, --start <n>:"
  echo -e "     Start the server #n"
  echo -e "  -sa, --startall:"
  echo -e "     Start the tracing server and all servers"
  echo -e "  -t, --trace:"
  echo -e "     Start tracing server"
  echo -e "  -gt, --gettrace:"
  echo -e "     Get trace logs from the tracing server, save in  current directory"
  echo -e "                        "
  echo -e "  -k, --kill <n>:"
  echo -e "     Kill the server #n, or 't' for tracing, 'c' for client"
  echo -e "  -ka, --killall:"
  echo -e "     Kill the tracing server, cloud client, and all servers"
  echo -e "  -kc, --killclient:"
  echo -e "     Kill the cloud client"
  echo -e "  -kt, --killtrace:"
  echo -e "     Kill tracing server"
  echo -e "                        "
  echo -e "  -p, --partition <n1, n2>:"
  echo -e "     Partition the server #n1 from the server #n2 and vice-versa"
  echo -e "  -up, --unpartition <n1, n2>:"
  echo -e "     Unpartition the server #n1 from the server #n2 and vice-versa"
  echo -e "  -upa, --unpartitionall:"
  echo -e "     Unpartition all servers by resetting IPTables"

}

#from https://stackoverflow.com/questions/192249/how-do-i-parse-command-line-arguments-in-bash
POSITIONAL_ARGS=()

while [[ $# -gt 0 ]]; do
  case $1 in
  -h | --help)
    usage
    exit 0
    ;;
  -u | --upload)
    UPLOAD=true
    filepath="$2"
    shift # past argument
    shift #past var
    ;;
  -sa | --start)
    START_ALL=true
    shift # past argument
    ;;
  -t | --tracing)
    START_TRACING=true
    shift
    ;;
  -kt | --killtrace)
    KILL_TRACE=true
    shift
    ;;
  -s | --server)
    START_SERVER=true
    servernum="$2"
    shift
    shift
    ;;
  -c | --client)
    START_CLIENT=true
    shift
    ;;
  -kc | --killclient)
    KILL_CLIENT=true
    shift
    ;;
  -k | --kill)
    KILL_ONE=true
    servernum="$2"
    shift
    shift
    ;;
  -ka | --killAll)
    KILL_ALL=true
    shift
    ;;
  -p | --Partition)
    PARTITION=true
    servernum1="$2"
    servernum2="$3"
    shift
    shift
    shift
    ;;
  -up | --Unpartition)
    UNPARTITION=true
    servernum1="$2"
    servernum2="$3"
    shift
    shift
    shift
    ;;
  -upa | --Unpartition-all)
    UNPARTITION_ALL=true
    shift
    ;;
  -i | --init)
    SETUP=true
    servernum="$2"
    shift
    shift
    ;;
  -r | --run)
    INTERACTIVE=true
    shift
    ;;
  -gt | --get_traces)
    GET_TRACES=true
    shift
    ;;
  --ssh)
    SSH=true
    servernum="$2"
    shift
    shift
    ;;
  -* | --*)
    echo "Unknown option $1"
    usage
    echo -e "${RED}Expected one of the above commands${NC}"
    exit 1
    ;;
  *)
    POSITIONAL_ARGS+=("$1") # save positional arg
    shift                   # past argument
    ;;
  esac
done

if [[ $START_ALL = true ]]; then
  startAll
elif [[ $UPLOAD = true ]]; then
  if [ -z "$filepath" ]; then
    echo -e "${RED}Error: Missing path parameter for uploading files ${NC}"
  else
    echo "Uploading all files in $filepath"
    uploadFiles "$filepath"
  fi
elif [[ $KILL_ALL = true ]]; then
  killAll
elif [[ $SETUP = true ]]; then
  if [ -z "$servernum" ]; then
    echo -e "${RED}Error: Missing server number parameter for setting up one server ${NC}"
  else
    setup "$servernum"
  fi
elif [[ $KILL_ONE = true ]]; then
  if [ -z "$servernum" ]; then
    echo -e "${RED}Error: Missing server number parameter for killing one server ${NC}"
  else
    killOneServer "$servernum"
  fi
elif [[ $UNPARTITION_ALL = true ]]; then
  unpartitionAll
elif [[ $PARTITION = true ]]; then
  if [ -z "$servernum1" ] || [ -z "$servernum2" ]; then
    echo -e "${RED}Error: Missing server number parameters for partitioning ${NC}"
  else
    partition "$servernum1" "$servernum2"
  fi
elif [[ $UNPARTITION = true ]]; then
  if [ -z "$servernum1" ] || [ -z "$servernum2" ]; then
    echo -e "${RED}Error: Missing server number parameters for unpartitioning ${NC}"
  else
    unpartition "$servernum1" "$servernum2"
  fi
elif [[ $START_TRACING = true ]]; then
  echo "Starting tracing server"
  trace
elif [[ $START_SERVER = true ]]; then
  if [ -z "$servernum" ]; then
    echo -e "${RED}Error: Missing server number parameter for starting one server ${NC}"
  else
    echo "Starting server $servernum"
    startOneServer "$servernum"
  fi
elif [[ $INTERACTIVE = true ]]; then
  interactive
elif [[ $GET_TRACES = true ]]; then
  getTrace
elif [[ $START_CLIENT = true ]]; then
  startClient
elif [[ $KILL_CLIENT = true ]]; then
  killClient
elif [[ $KILL_TRACE = true ]]; then
  killTrace
elif [[ $SSH = true ]]; then
  if [ -z "$servernum" ]; then
    echo -e "${RED}Error: Missing server number parameter for ssh ${NC}"
  else
    ssh "$servernum"
  fi
else
  echo -e "${RED}Error: Missing parameter. Expected one of -h | -u | -sa | -t | -kt | -s | -c | -kc | k | -ka | -p | -up | -upa | -i | -r | -gt | --ssh | ${NC}"
fi
