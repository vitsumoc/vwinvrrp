// 类似VRRP的功能
// 双机热备, 共享一个虚地址, 备机只有当主机不服务时才使用虚地址
package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	fastping "github.com/tatsushid/go-fastping"
	"gopkg.in/ini.v1"
)

// 配置文件
var isMaster bool
var masterPort string
var ifName string
var ifMAddr string
var ifVAddr string
var ifVMask string
var ifVGw string
var masterRefresh bool

func main() {
	// 解析配置文件
	cfg, err := ini.Load("Conf.ini")
	if err != nil {
		fmt.Printf("Fail to load config file: %v", err)
		return
	}
	isMaster, _ = strconv.ParseBool(cfg.Section("VRRP").Key("IS_MASTER").In("true", []string{"true", "false"}))
	masterPort = cfg.Section("VRRP").Key("MASTER_PORT").String()
	ifName = cfg.Section("VRRP").Key("IF_NAME").String()
	ifMAddr = cfg.Section("VRRP").Key("IF_M_ADDR").String()
	ifVAddr = cfg.Section("VRRP").Key("IF_V_ADDR").String()
	ifVMask = cfg.Section("VRRP").Key("IF_V_MASK").String()
	ifVGw = cfg.Section("VRRP").Key("IF_V_GW").String()

	// 对于主机来说，只要在重启后完成虚地址配置(重复配置也无妨)
	if isMaster {
		masterProcess()
	}
	// 对于备机来说
	if !isMaster {
		slaveProcess()
	}
}

// 主机需要考虑的事情
func masterProcess() {
	masterRefresh = false
	// 当备机让位时, 主机需要收到通知, 重新配置一次 IP
	r := gin.Default()
	r.GET("/youaremaster", func(c *gin.Context) {
		masterRefresh = true
		c.JSON(http.StatusOK, gin.H{
			"msg": "ok",
		})
	})
	go r.Run("0.0.0.0:" + masterPort)
	// 正常就是抢占虚拟IP即可
	cmd := exec.Command("cmd", "/c", "netsh", "interface", "ip", "add", "address",
		"name="+ifName, "addr="+ifVAddr, "mask="+ifVMask, "gateway="+ifVGw)
	cmd.Run() // 这里不处理报错是因为可能重复添加
	// 然后进程卡着就行了
	for {
		time.Sleep(2 * time.Second)
		if masterRefresh {
			time.Sleep(1 * time.Second)
			// 删除
			cmd := exec.Command("cmd", "/c", "netsh", "interface", "ip", "delete", "address",
				"name="+ifName, "addr="+ifVAddr)
			cmd.Run()
			time.Sleep(1 * time.Second)
			// 添加
			cmd2 := exec.Command("cmd", "/c", "netsh", "interface", "ip", "add", "address",
				"name="+ifName, "addr="+ifVAddr, "mask="+ifVMask, "gateway="+ifVGw)
			cmd2.Run()
			// 标记
			masterRefresh = false
		}
	}
}

// 备机需要考虑的事情
func slaveProcess() {
	// 切换标志
	_isMaster := false // 当前为主机的标志
	_failCount := 0    // 标记没有收到回复(失败)的次数，n次不收到回复则将自己切换为虚ip拥有者
	// 先避免自己拥有虚拟ip
	cmd := exec.Command("cmd", "/c", "netsh", "interface", "ip", "delete", "address",
		"name="+ifName, "addr="+ifVAddr)
	cmd.Run() // 这里不处理报错是因为可能重复删除

	p := fastping.NewPinger()
	ra, err := net.ResolveIPAddr("ip4:icmp", ifMAddr)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	p.AddIPAddr(ra)
	// 在收到回复时触发
	p.OnRecv = func(addr *net.IPAddr, rtt time.Duration) {
		// 失败次数归零
		_failCount = 0
		// 将自己切为备机
		if _isMaster {
			cmd := exec.Command("cmd", "/c", "netsh", "interface", "ip", "delete", "address",
				"name="+ifName, "addr="+ifVAddr)
			cmd.Run() // 这里不处理报错是因为可能重复删除
			_isMaster = false
			// 考虑到主机重启后, 启动 VRRP 服务需要时间
			// 这里多次请求，直到成功
			for {
				time.Sleep(5 * time.Second)
				res, err := http.Get("http://" + ifMAddr + ":" + masterPort + "/youaremaster")
				if err != nil {
					continue
				}
				if res.StatusCode != http.StatusOK {
					res.Body.Close()
					continue
				}
				// 返回成功了，不用重试了
				res.Body.Close()
				break
			}
		}
	}
	// 无论是否收到回复, 每次结束时触发
	p.OnIdle = func() {
		if !_isMaster {
			_failCount += 1
			if _failCount >= 4 {
				cmd := exec.Command("cmd", "/c", "netsh", "interface", "ip", "add", "address",
					"name="+ifName, "addr="+ifVAddr, "mask="+ifVMask, "gateway="+ifVGw)
				cmd.Run()
				_isMaster = true
			}
		}
	}
	// 无限长ping
	for {
		time.Sleep(1 * time.Second)
		err = p.Run()
		if err != nil {
			fmt.Println(err)
		}
	}
}
