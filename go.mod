module github.com/realli07kkk/sentinel-sync-dns

go 1.20

require (
	github.com/aliyun/alibaba-cloud-sdk-go v1.63.107
	github.com/go-redis/redis/v8 v8.11.5
	github.com/huaweicloud/huaweicloud-sdk-go-v3 v0.1.128
	github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common v1.0.1068
	github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/privatedns v1.0.1068
	gopkg.in/yaml.v2 v2.4.0
)

require (
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/opentracing/opentracing-go v1.2.1-0.20220228012449-10b1cf09e00b // indirect
	github.com/tjfoc/gmsm v1.4.1 // indirect
	go.mongodb.org/mongo-driver v1.12.0 // indirect
	golang.org/x/crypto v0.23.0 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// 移除旧版本冲突
replace github.com/tencentcloud/tencentcloud-sdk-go => github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud v1.0.1068
