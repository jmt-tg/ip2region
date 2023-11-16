package main

import (
	"database/sql/driver"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	gin "github.com/gin-gonic/gin"
	"github.com/lionsoul2014/ip2region/binding/golang/xdb"
	"log"
	"strings"
)

var port = flag.String("p", "8080", "port")

// 中国大陆的省份，不包括香港","澳门","台湾
var chinaInlandProvince = []string{
	"河北", "山西", "黑龙江", "吉林", "辽宁", "江苏", "浙江", "安徽", "福建", "江西", "山东", "河南", "湖北", "湖南", "广东", "海南", "四川", "贵州", "云南", "陕西", "甘肃", "青海", "内蒙古", "广西", "西藏", "宁夏", "新疆", "北京", "天津", "上海", "重庆",
}

func getRegion(context *gin.Context) (bool, Obj) {
	var cliIp string
	if context.GetHeader("X-REAL-IP") != "" {
		cliIp = context.GetHeader("X-REAL-IP")
	} else if context.GetHeader("X-FORWARDED-FOR") != "" {
		cliIp = context.GetHeader("X-FORWARDED-FOR")
	} else {
		cliIp = context.ClientIP()
	}
	// 去除端口
	cliIp = strings.Split(cliIp, ":")[0]
	region := Ip2Region(cliIp)
	// 中国大陆的省份，不包括香港","澳门","台湾
	isChainInland := false
	for _, province := range chinaInlandProvince {
		if strings.Contains(region.Province, province) {
			isChainInland = true
			break
		}
	}
	return isChainInland, region
}

func main() {
	flag.Parse()
	engine := gin.Default()
	engine.GET("/", func(context *gin.Context) {
		isChainInland, region := getRegion(context)
		context.JSON(200, gin.H{
			"code":    0,
			"msg":     "success",
			"data":    region,
			"isChina": isChainInland,
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
				return Obj{regionSplit[0], regionSplit[1], regionSplit[2], regionSplit[3], regionSplit[4]}
			}
		}
	}
	return Obj{}
}
