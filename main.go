package main

import (
	"net/http"
	qhy_url "net/url"
	"fmt"
	"math/rand"
	"time"
	"io"
	"github.com/PuerkitoBio/goquery"
	"log"
	"strings"
	"io/ioutil"
	"encoding/json"
	"os"
)

//随拾爬虫

var (
	//更多分类接口地址
	moreCategoryUrl = "https://www.zhihu.com/node/TopicsPlazzaListV2"

	//访问页面
	url = "https://www.zhihu.com/topics"

	done chan bool

	Jobs chan TagResult
)

type Category struct {
	name 		string

	DataId      string
}

//爬虫的目标结果
type TagResult struct {
	Category 	string

	Title 		string

	Desc 		string

	ImgUrl 		string
}

//伪造User-Agent
var userAgent = [...]string{"Mozilla/5.0 (compatible, MSIE 10.0, Windows NT, DigExt)",
	"Mozilla/4.0 (compatible, MSIE 7.0, Windows NT 5.1, 360SE)",
	"Mozilla/4.0 (compatible, MSIE 8.0, Windows NT 6.0, Trident/4.0)",
	"Mozilla/5.0 (compatible, MSIE 9.0, Windows NT 6.1, Trident/5.0,",
	"Opera/9.80 (Windows NT 6.1, U, en) Presto/2.8.131 Version/11.11",
	"Mozilla/4.0 (compatible, MSIE 7.0, Windows NT 5.1, TencentTraveler 4.0)",
	"Mozilla/5.0 (Windows, U, Windows NT 6.1, en-us) AppleWebKit/534.50 (KHTML, like Gecko) Version/5.1 Safari/534.50",
	"Mozilla/5.0 (Macintosh, Intel Mac OS X 10_7_0) AppleWebKit/535.11 (KHTML, like Gecko) Chrome/17.0.963.56 Safari/535.11",
	"Mozilla/5.0 (Macintosh, U, Intel Mac OS X 10_6_8, en-us) AppleWebKit/534.50 (KHTML, like Gecko) Version/5.1 Safari/534.50",
	"Mozilla/5.0 (Linux, U, Android 3.0, en-us, Xoom Build/HRI39) AppleWebKit/534.13 (KHTML, like Gecko) Version/4.0 Safari/534.13",
	"Mozilla/5.0 (iPad, U, CPU OS 4_3_3 like Mac OS X, en-us) AppleWebKit/533.17.9 (KHTML, like Gecko) Version/5.0.2 Mobile/8J2 Safari/6533.18.5",
	"Mozilla/4.0 (compatible, MSIE 7.0, Windows NT 5.1, Trident/4.0, SE 2.X MetaSr 1.0, SE 2.X MetaSr 1.0, .NET CLR 2.0.50727, SE 2.X MetaSr 1.0)",
	"Mozilla/5.0 (iPhone, U, CPU iPhone OS 4_3_3 like Mac OS X, en-us) AppleWebKit/533.17.9 (KHTML, like Gecko) Version/5.0.2 Mobile/8J2 Safari/6533.18.5",
	"MQQBrowser/26 Mozilla/5.0 (Linux, U, Android 2.3.7, zh-cn, MB200 Build/GRJ22, CyanogenMod-7) AppleWebKit/533.1 (KHTML, like Gecko) Version/4.0 Mobile Safari/533.1"}

var r = rand.New(rand.NewSource(time.Now().UnixNano()))
func GetRandomUserAgent() string {
	return userAgent[r.Intn(len(userAgent))]
}

//爬取
//io.ReadCloser，不需要时，需要关闭
//defer body.Close()
func Worm(url string) (io.ReadCloser, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("页面访问出错 ", err)
	}
	req.Header.Set("User-Agent", GetRandomUserAgent())

	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Get请求%s返回错误:%s", url, err)
	}
	if res.StatusCode == 200 {
		body := res.Body
		return body, nil
	}
	return nil, fmt.Errorf("网页不存在")
}

//处理
func WormJob(category Category, done chan<- bool) error {
	defer func() {
		done <- true
		fmt.Println(category, "分类处理完毕")

		if r := recover(); r != nil {
			log.Println("[E]", r)
		}
	}()

	fmt.Println("处理", category, "分类")
	urlCategory := url + category.name
	body, err := Worm(urlCategory)
	if err != nil {
		return err
	}
	defer body.Close()

	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return err
	}

	doc.Find("div.item").Each(func(i int,s *goquery.Selection) {
		s.Children().Each(func(j int,ss *goquery.Selection) {
			result := TagResult{}

			imgUrl, ok := ss.Find("img").Attr("src")
			if ok {
			//	fmt.Println("img_url=", imgUrl)
				result.ImgUrl = imgUrl
			}

			title := ss.Find("strong").Text()
			if len(title) > 0 {
				//fmt.Println("title=", title)
				result.Title = title
			}

			desc := ss.Find("p").Text()
			if len(desc) > 0 {
				//fmt.Println("desc=", desc)
				result.Desc = desc
			}
			result.Category = category.name

			Jobs <- result

		})
	})

	//请求"更多"里面的数据
	offset := 0
	for {
		response, err := MoreWormJob(category, offset)
		if err != nil {
			fmt.Println(err)
			break;
		}

		if response <= 0 {
			break;
		}

		offset += response
	}
	fmt.Println(category.name, "请求更多数据条数： ", offset)


	return nil
}

//抓取"更多"分类里面的数据，返回请求的条数
func MoreWormJob(category Category, offset int) (int, error) {
	topic := category.DataId
	param := fmt.Sprintf("{\"topic_id\":%s,\"offset\":%d,\"hash_id\":\"\"}", topic, offset)
	data := make(qhy_url.Values)
	data["method"] = []string{"next"}
	data["params"] = []string{param}
	res, err := http.PostForm(moreCategoryUrl, data)
	if err != nil {
		return 0, fmt.Errorf("加载更多页面出错 ", err)
	}
	res.Header.Set("User-Agent", GetRandomUserAgent())
	defer res.Body.Close()
	body2, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return 0, err
	}
	jsonData := string(body2)

	//json解析
	var count int //条数
	var dt map[string]interface{}
	json.Unmarshal([]byte(jsonData), &dt)

	if web, ok := dt["msg"]; ok {
		msgs := web.([]interface{})
		htl := ""
		for _, div := range msgs {
			htl += div.(string)
		}
		count = len(msgs)
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(htl))
		if err != nil {
			return 0, fmt.Errorf("更多doc", err)
		}
		doc.Find("div.item").Each(func(i int,s *goquery.Selection) {
			s.Children().Each(func(j int,ss *goquery.Selection) {
				result := TagResult{}

				imgUrl, ok := ss.Find("img").Attr("src")
				if ok {
					//	fmt.Println("img_url=", imgUrl)
					result.ImgUrl = imgUrl
				}

				title := ss.Find("strong").Text()
				if len(title) > 0 {
					//fmt.Println("title=", title)
					result.Title = title
				}

				desc := ss.Find("p").Text()
				if len(desc) > 0 {
					//fmt.Println("desc=", desc)
					result.Desc = desc
				}
				result.Category = category.name

				Jobs <- result
			})
		})
	}

	return count, nil
}

func main() {

	startTime := time.Now()

	defer func() {
		endTime := time.Now()
		tt := endTime.Sub(startTime)
		fmt.Println("爬数据时间总共为：", tt)
	}()

	body, err := Worm(url)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer body.Close()
/*	bodyByte, _ := ioutil.ReadAll(body)
	resStr := string(bodyByte)
	fmt.Println(resStr)*/

	Jobs = make(chan TagResult, 20000)

	//读取分类大标题
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		log.Fatal(err)
	}

	var bigCategory []Category
	doc.Find("ul.zm-topic-cat-main").Each(func(i int,s *goquery.Selection) { //获取节点集合并遍历
		s.Children().Each(func(j int,ss *goquery.Selection) {
			category,_ := ss.Find("a").Attr("href")
			categorys := strings.Split(category, " ")

			topic, _ := ss.Attr("data-id")
			bigCategory = append(bigCategory, Category{categorys[0], topic})
		})

	})

	//fmt.Println(bigCategory)
	if len(bigCategory) <= 0 {
		log.Println("没有爬到数据");
		return;
	}

	done = make(chan bool, len(bigCategory))
	//循环大分类
	for _, t := range bigCategory {
		go WormJob(t, done)
	}

	//等待所有的工作做完
	go func() {
		for i := 0; i < len(bigCategory); i++ {
			<-done
		}
		close(Jobs)
	}()

	//处理工作
	//计数器
	count := 1
	fileName := "category.toml"
	os.Remove(fileName)
	file, err := os.OpenFile(fileName, os.O_WRONLY | os.O_CREATE | os.O_APPEND, 0644)
	if err != nil {
		fmt.Println(err)
	}
	defer file.Close()

	for result := range Jobs {
		fmt.Printf("============%d================\n",count)
		fmt.Println("category=", result.Category)
		fmt.Println("img_url=", result.ImgUrl)
		fmt.Println("title=", result.Title)
		fmt.Println("desc=", result.Desc)
		fmt.Printf("============%d===============\n", count)
		count += 1

		//写入文件
		if _, err := file.Write([]byte("category=" + result.Category+"\r\n")); err != nil {
			fmt.Println(err)
		}

		if _, err := file.Write([]byte("img_url=" + result.ImgUrl + "\r\n")); err != nil {
			fmt.Println(err)
		}

		if _, err := file.Write([]byte("title=" + result.Title + "\r\n")); err != nil {
			fmt.Println(err)
		}

		if _, err := file.Write([]byte("desc=" + result.Desc + "\r\n\r\n")); err != nil {
			fmt.Println(err)
		}


	}



	log.Println("处理完成")

}
