package main

import (
	"database/sql/driver"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	gin "github.com/gin-gonic/gin"
	"github.com/lionsoul2014/ip2region/binding/golang/xdb"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

var port = flag.String("p", "8080", "port")

// 中国大陆的省份，不包括香港","澳门","台湾
var chinaOutlandProvince = []string{
	"香港",
	"澳门",
	"台湾",
}

func getRegion(context *gin.Context) (bool, bool, Obj) {
	var cliIp = context.Query("ip")
	if cliIp == "" {
		if context.GetHeader("X-REAL-IP") != "" {
			cliIp = context.GetHeader("X-REAL-IP")
		} else if context.GetHeader("X-FORWARDED-FOR") != "" {
			cliIp = context.GetHeader("X-FORWARDED-FOR")
		} else {
			cliIp = context.ClientIP()
		}
	}
	// 去除端口
	cliIp = strings.Split(cliIp, ":")[0]
	region := Ip2Region(cliIp)
	// 中国大陆的省份，不包括香港","澳门","台湾
	isChina := false
	isChainInland := true
	if region.Country == "中国" {
		isChina = true
	} else if region.Province == "中国" {
		isChina = true
	}
	if isChina == false {
		isChainInland = false
	}
	for _, v := range chinaOutlandProvince {
		if strings.Contains(region.Country, v) {
			isChainInland = false
			break
		} else if strings.Contains(region.Province, v) {
			isChainInland = false
			break
		} else if strings.Contains(region.City, v) {
			isChainInland = false
			break
		} else if strings.Contains(region.District, v) {
			isChainInland = false
			break
		} else if strings.Contains(region.ISP, v) {
			isChainInland = false
			break
		}
	}
	return isChina, isChainInland, region
}

func main() {
	flag.Parse()
	engine := gin.Default()
	engine.GET("/", func(context *gin.Context) {
		isChina, isChinaInland, region := getRegion(context)
		context.JSON(200, gin.H{
			"code":          0,
			"msg":           "success",
			"data":          region,
			"isChinaCounty": isChina,
			"isChinaInland": isChinaInland,
			"isChina":       isChinaInland,
		})
	})
	fmt.Printf("listen on http://127.0.0.1:%s\n", *port)
	engine.Run(":" + *port)
}

//go:embed ip2region.xdb
var Ip2regionXdb []byte

var (
	_searcher *xdb.Searcher
)

func init() {
	var err error
	_searcher, err = xdb.NewWithBuffer(Ip2regionXdb)
	if err != nil {
		log.Fatalf("open xdb file error: %v", err)
	}
	Ip2Region("114.114.114.114")
}

type Obj struct {
	Country  string `json:"country" bson:"country"`
	District string `json:"district" bson:"district"`
	Province string `json:"province" bson:"province"`
	City     string `json:"city" bson:"city"`
	ISP      string `json:"isp" bson:"isp"`
}

func (o Obj) Value() (driver.Value, error) {
	return json.Marshal(o)
}

func (o *Obj) Scan(src interface{}) error {
	return json.Unmarshal(src.([]byte), o)
}

func (o Obj) String() string {
	return o.Country + "|" + o.District + "|" + o.Province + "|" + o.City + "|" + o.ISP
}

func Ip2Region(ip string) Obj {
	if ip == "" {
		return Obj{}
	}
	split := strings.Split(ip, ",")
	for _, s := range split {
		str, _ := _searcher.SearchByStr(s)
		if str != "" {
			regionSplit := strings.Split(str, "|")
			if len(regionSplit) == 5 {
				if regionSplit[0] == "0" || regionSplit[0] == "" || regionSplit[0] == "内网" || regionSplit[4] == "内网IP" {
					return Obj{regionSplit[0], regionSplit[1], regionSplit[2], regionSplit[3], regionSplit[4]}
				}
				//return Obj{regionSplit[0], regionSplit[1], regionSplit[2], regionSplit[3], regionSplit[4]}
				api := getByApi(s)
				if api.Country != "" && !strings.HasPrefix(api.Country, "$") {
					return api
				}
				return Obj{regionSplit[0], regionSplit[1], regionSplit[2], regionSplit[3], regionSplit[4]}
			}
		}
	}
	return Obj{}
}

var addressPrefix = "http://43.129.178.179:5168/api/ip2region/getregion?ip="

func getByApi(ip string) Obj {
	// 判断是不是3个.
	if strings.Count(ip, ".") != 3 {
		return Obj{}
	}
	// 判断ip是否符合规范
	ipV4Reg := `^((25[0-5]|2[0-4]\d|1\d{2}|[1-9]\d|\d)\.){3}(25[0-5]|2[0-4]\d|1\d{2}|[1-9]\d|\d)$`
	r := regexp.MustCompile(ipV4Reg)
	if !r.MatchString(ip) {
		return Obj{}
	}
	// 判断是否是内网ip
	if strings.HasPrefix(ip, "10.") || strings.HasPrefix(ip, "192.168.") || strings.HasPrefix(ip, "172.") {
		return Obj{}
	}
	// 缓存取
	cacheMu.RLock()
	obj, ok := cache[ip]
	cacheMu.RUnlock()
	if ok {
		return obj
	}
	select {
	case <-time.After(3 * time.Second):
		return Obj{}
	default:
		request, _ := http.NewRequest("GET", fmt.Sprintf(
			`%s%s`, addressPrefix, ip,
		), nil)
		// set header
		//request.Header.Set("X-APISPACE-TOKEN", "4rfuyxgx2rjefi57r6nfqavxuprjv9z8")
		// send request
		client := &http.Client{}
		response, err := client.Do(request)
		if err != nil {
			log.Printf("getByApi error: %v", err)
			return Obj{}
		}
		defer response.Body.Close()
		// read response
		respBs, err := io.ReadAll(response.Body)
		if err != nil {
			log.Printf("getByApi error: %v", err)
			return Obj{}
		}
		s := string(respBs)
		// 解析，格式为 大陆|国家|省份|城市
		regionSplit := strings.Split(s, "|")
		switch len(regionSplit) {
		case 4:
			obj = Obj{
				Country:  regionSplit[1],
				Province: regionSplit[2],
				City:     regionSplit[3],
			}
		case 3:
			obj = Obj{
				Country:  regionSplit[1],
				Province: regionSplit[2],
			}
		case 2:
			obj = Obj{
				Country: regionSplit[1],
			}
		case 1:
			obj = Obj{Country: regionSplit[0]}
		default:
			obj = Obj{}
		}
		cacheMu.Lock()
		cache[ip] = obj
		cacheMu.Unlock()
		return obj
	}
}

var cache = make(map[string]Obj)
var cacheMu sync.RWMutex

func init() {
	v := os.Getenv("IP2REGION_API_ADDRESS")
	if v != "" {
		addressPrefix = v
	}
	ticker := time.NewTicker(10 * time.Minute)
	// 清理缓存 限制缓存大小为1w
	go func() {
		for range ticker.C {
			cacheMu.Lock()
			if len(cache) > 10000 {
				cache = make(map[string]Obj)
			}
			cacheMu.Unlock()
		}
	}()
}
