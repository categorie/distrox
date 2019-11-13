package proxy

import (
    "bufio"
    "encoding/json"
    "fmt"
    "io/ioutil"
    "log"
    "os"
    "bytes"
    "io"
    "strings"
    "strconv"
    "net"
)

type ProxyConfig struct {
    LeaderId         int         `json:"leader_id"`
    BlockedSitesPath string      `json:"blocked_sites_path"`
    Nodes            []*NodeInfo `json:"nodes"`
}

type NodeInfo struct {
    Host     string `json:"host"`
    Port     int    `"json:"port"`
    Url      string
    IsLeader bool
}

type ProxyNode struct {
    BlockedSites   map[string]string
    Info           *NodeInfo
    PeerInfo       []*NodeInfo
    SendingPeerIdx int
    //Cache          *LocalCache
    Messenger *TCPMessenger
}

func CreateProxyNode(host string, port int, leader bool) *ProxyNode {
    rv := &ProxyNode{}
    rv.BlockedSites = make(map[string]string)

    rv.SendingPeerIdx = 0
    rv.Info = CreateNodeInfo(host, port, leader)
    rv.Messenger = InitTCPMessenger(rv.Info.Url)
    return rv
}

func CreateNodeInfo(host string, port int, leader bool) *NodeInfo {
    rv := &NodeInfo{}
    rv.Host = host
    rv.Port = port
    rv.Url = fmt.Sprintf("%s:%d", host, port)
    rv.IsLeader = leader
    return rv
}

func ReadConfig(path string) ProxyConfig {
    var config ProxyConfig
    file, err := ioutil.ReadFile(path)
    if err != nil {
	log.Fatal(err)
    }

    err = json.Unmarshal([]byte(file), &config)
    if err != nil {
	log.Fatal(err)
    }

    for _, node := range config.Nodes {
	node.Url = fmt.Sprintf("%s:%d", node.Host, node.Port)
	/*
	if node.Id == config.LeaderId {
	    node.IsLeader = true
	} else {
	    node.IsLeader = false
	}
	*/
    }
    return config
}

func (p *ProxyNode) ReadBlockedSites(path string) {
    file, err := os.Open(path)
    if err != nil {
	log.Fatal(err)
    }
    defer file.Close()

    scanner := bufio.NewScanner(file)
    for scanner.Scan() {
	site := scanner.Text()
	p.BlockedSites[site] = site
    }
}

func (p *ProxyNode) HandleRequests() {
    l := p.Messenger.Listener
    for {
	conn, err := l.Accept()
	if err != nil {
	    log.Fatal(err)
	}
	var buf bytes.Buffer
	io.Copy(&buf, conn)
	b := buf.Bytes()
	go p.HandleRequest(b)
	conn.Close()
    }
}

func (p *ProxyNode) HandleRequest(b []byte) {
    message := BytesToMessage(b)
    message_hash := HashBytes(message.Data)

    p.Messenger.PruneStoredMessages()
    message_found := p.Messenger.HasMessageStored(message_hash)

    if !message_found && message.SenderUrl != p.Info.Url {
	p.Messenger.RecentMessageHashes[message_hash] = message.Timestamp

	if message.MessageType == MULTICAST_MESSAGE {
	    println(string(message.Data))
	    p.Multicast(b)
	} else if message.MessageType == JOIN_REQUEST_MESSAGE {
	    m := string(message.Data)
	    tokens := strings.Split(m, ":")
	    port, _ := strconv.Atoi(tokens[1])
	    new_node_info := CreateNodeInfo(tokens[0], port, false)
	    log.Printf("New node joined with URL %s!", new_node_info.Url)
	    p.PeerInfo = append(p.PeerInfo, new_node_info)

	    // notify all the nodes in the group of the new node joining
	    nodes_string := p.ConstructAllNodesString()
	    msg := CreateMessage([]byte(nodes_string), p.Info.Url, JOIN_RESPONSE_MESSAGE)
	    p.Multicast(MessageToBytes(msg))

	} else if message.MessageType == JOIN_RESPONSE_MESSAGE {
	    p.Multicast(b)
	    peer_infos := strings.Split(string(message.Data), " ")
	    for _, info := range peer_infos {
		tokens := strings.Split(info, ":")
		port, _ := strconv.Atoi(tokens[1])
		url := fmt.Sprintf("%s:%d", tokens[0], port);

		if url == p.Info.Url {
		    continue
		}
		if !p.ContainsUrl(url) {
		    new_node_info := CreateNodeInfo(tokens[0], port, false)
		    p.PeerInfo = append(p.PeerInfo, new_node_info)
		    log.Printf("New node joined with URL %s!", new_node_info.Url)
		}
	    }

	} else if message.MessageType == UNICAST_MESSAGE {
	    println(string(message.Data))
	}
    }
}

func (p *ProxyNode) Unicast(message []byte, url string) {
    conn, err := net.Dial("tcp", url)
    if err != nil {
	log.Fatal(err)
    }

    defer conn.Close()

    _, err = conn.Write(message)
    if err != nil {
	log.Fatal(err)
    }
    return;
}

func (p *ProxyNode) Multicast(message []byte) {
    for _, info := range p.PeerInfo {
	url := info.Url
	conn, err := net.Dial("tcp", url)
	if err != nil {
	    log.Fatal(err.Error())
	}
	_, err = conn.Write(message)
	if err != nil {
	    log.Fatal(err.Error())
	}
	conn.Close()
    }
}

func (p *ProxyNode) ConstructAllNodesString() string {
    rv := p.Info.Url
    for _, info := range p.PeerInfo {
	rv += " "
	rv += info.Url
    }
    return rv
}

func (p *ProxyNode) ContainsUrl(url string ) bool {
    for _, info := range p.PeerInfo {
	if url == info.Url {
	    return true
	}
    }
    return false
}
