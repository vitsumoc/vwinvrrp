// 类似VRRP的功能
// 双机热备, 共享一个虚地址, 备机只有当主机不服务时才使用虚地址
package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"time"

	fastping "github.com/tatsushid/go-fastping"
	"gopkg.in/ini.v1"
)

func main() {
	cfg, err := ini.Load("Conf.ini")
	if err != nil {
		fmt.Printf("Fail to load config file: %v", err)
		return
	}

	// 配置文件
	isMaster, _ := strconv.ParseBool(cfg.Section("VRRP").Key("IS_MASTER").In("true", []string{"true", "false"}))
	ifName := cfg.Section("VRRP").Key("IF_NAME").String()
	ifMAddr := cfg.Section("VRRP").Key("IF_M_ADDR").String()
	ifVAddr := cfg.Section("VRRP").Key("IF_V_ADDR").String()
	ifVMask := cfg.Section("VRRP").Key("IF_V_MASK").String()
	ifVGw := cfg.Section("VRRP").Key("IF_V_GW").String()

	// 对于主机来说，只要在重启后完成虚地址配置(重复配置也无妨)
	if isMaster {
		cmd := exec.Command("cmd", "/c", "netsh", "interface", "ip", "add", "address",
			"name="+ifName, "addr="+ifVAddr, "mask="+ifVMask, "gateway="+ifVGw)
		cmd.Run() // 这里不处理报错是因为可能重复添加
		// 然后进程卡着就行了
		for {
			time.Sleep(120 * time.Second)
		}
	}

	// 对于备机来说
	if !isMaster {
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
}
