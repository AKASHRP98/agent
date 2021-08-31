package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os/exec"

	"github.com/labstack/echo/v4"
)

func configNuster() error {

	// read file
	// data, err := ioutil.ReadFile("/usr/Hosting/config.json")
	// if err != nil {
	// 	return echo.NewHTTPError(404, "Config file not found")
	// }

	// // json data
	// var obj Config

	// // unmarshall it
	// err = json.Unmarshal(data, &obj)
	// if err != nil {
	// 	return echo.NewHTTPError(400, err)
	// }
	log.Print(obj)
	conf := "##############################\n# Do not edit this file#\n#############################\n"

	conf = conf + fmt.Sprintf("global\n\tnuster cache off data-size %dm\n\tmaster-worker\n\tmaxconn %d\n\ttune.ssl.default-dh-param 2048\n\tssl-dh-param-file /opt/Hosting/dhparam.pem\n", obj.Global.Datasize, obj.Global.Maxconn)

	conf = conf + fmt.Sprintf("defaults\n\tmode http\n\ttimeout connect %ds\n\ttimeout client %ds\n\ttimeout server %ds", obj.Default.Timeout.Connect, obj.Default.Timeout.Client, obj.Default.Timeout.Server)

	if len(obj.Sites) == 0 {
		conf = conf + `
http-errors myerrors
    errorfile 503 /usr/Hosting/errors/404.http`
	}

	conf = conf + `
frontend nonssl
    bind *:80`

	if obj.SSL {
		conf = conf + `
	bind *:443 ssl crt /opt/Hosting/certs/ alpn h2,http/1.1
	http-request set-header X-Forwarded-Proto https if { ssl_fc }`
	}
	for _, frontend := range obj.Sites {
		if frontend.PrimaryDomain.Routing == "www" {
			conf = conf + fmt.Sprintf(`
	redirect prefix http://www.%s code 301 if { hdr(host) -i %s }`, frontend.PrimaryDomain.Url, frontend.PrimaryDomain.Url)
		}
		if frontend.PrimaryDomain.Routing == "root" {
			conf = conf + fmt.Sprintf(`	
	redirect prefix http://%s code 301 if { hdr(host) -i www.%s }`, frontend.PrimaryDomain.Url, frontend.PrimaryDomain.Url)
		}
		for _, alias := range frontend.AliasDomain {
			if alias.Routing == "www" {
				conf = conf + fmt.Sprintf(` 
	redirect prefix http://www.%s code 301 if { hdr(host) -i %s }`, alias.Url, alias.Url)
			}
			if alias.Routing == "root" {
				conf = conf + fmt.Sprintf(`
	redirect prefix http://%s code 301 if { hdr(host) -i www.%s }`, alias.Url, alias.Url)
			}
		}
	}

	if len(obj.Sites) == 0 {
		conf = conf + `
	errorfiles myerrors
    http-response return status 404 default-errorfiles`
	}

	conf = conf + `
    acl has_cookie hdr_sub(cookie) wordpress_logged_in
    acl has_path path_sub wp-admin || wp-login
    acl static_file path_end .js || .css || .png || .jpg || .jpeg || .gif || .ico`

	conf = conf + `
    use_backend nocache if has_path || has_cookie
    use_backend static if static_file`
	if len(obj.Sites) != 0 {
		conf = conf + `
	use_backend %[req.hdr(host),map(/opt/Hosting/routes.map)] if { req.hdr(host),map(/opt/Hosting/routes.map) -m found }
	use_backend %[req.hdr(host),map_sub(/opt/Hosting/wildcardroutes.map)] if { req.hdr(host),map_sub(/opt/Hosting/wildcardroutes.map) -m found }`
	}
	for i, backend := range obj.Sites {

		conf = conf + fmt.Sprintf(`
backend %s
    nuster cache %s
    nuster rule r%d
    http-response set-header x-cache HIT if { nuster.cache.hit }
    http-response set-header x-cache MISS unless { nuster.cache.hit }
    server s1 0.0.0.0:8088`, backend.Name, backend.Cache, i)
	}
	conf = conf + `
backend nocache
    http-response set-header x-cache BYPASS
    server s2 0.0.0.0:8088
backend static
    http-response set-header x-type STATIC
    server s2 0.0.0.0:8088`

	conf = conf + "\n"
	// the WriteFile method returns an error if unsuccessful
	err := ioutil.WriteFile("/opt/Hosting/hosting.cfg", []byte(conf), 0777)
	wildSite := "###################### DO NOT EDIT THIS FILE #######################\n"
	appendSite := "##################### DO NOT EDIT THIS FILE #########################\n"
	for _, site := range obj.Sites {
		appendSite = appendSite + fmt.Sprintf("%s %s \n", site.PrimaryDomain.Url, site.Name)
		if site.PrimaryDomain.WildCard {
			wildSite = wildSite + fmt.Sprintf(".%s %s \n", site.PrimaryDomain.Url, site.Name)
		}
		if !site.PrimaryDomain.SubDomain {
			appendSite = appendSite + fmt.Sprintf("www.%s %s \n", site.PrimaryDomain.Url, site.Name)
		}
		for _, alias := range site.AliasDomain {
			appendSite = appendSite + fmt.Sprintf("%s %s \n", alias.Url, site.Name)
			if alias.WildCard {
				wildSite = wildSite + fmt.Sprintf(".%s %s \n", alias.Url, site.Name)
			}
			if !alias.SubDomain {
				appendSite = appendSite + fmt.Sprintf("www.%s %s \n", alias.Url, site.Name)
			}
		}
	}
	ioutil.WriteFile("/opt/Hosting/routes.map", []byte(appendSite), 0777)
	ioutil.WriteFile("/opt/Hosting/wildcardroutes.map", []byte(wildSite), 0777)
	// handle this error
	if err != nil {
		// print it out
		return echo.NewHTTPError(404, err)
	}

	return nil

}

func addSiteToJSON(wp wpadd) error {
	// read file
	// data, err := ioutil.ReadFile("/usr/Hosting/config.json")
	// if err != nil {
	// 	return echo.NewHTTPError(404, "Config file not found")
	// }

	// // json data
	// var obj Config

	// // unmarshall it
	// err = json.Unmarshal(data, &obj)
	// if err != nil {
	// 	return echo.NewHTTPError(400, "JSON data error")
	// }
	obj.Sites = wp.Sites
	newSite := Site{Name: wp.AppName, Cache: "off", Exclude: wp.Exclude}
	newSite.AliasDomain = []Domain{}

	newSite.PrimaryDomain = Domain{Url: wp.Url, SSL: false, SubDomain: wp.SubDomain, Routing: wp.Routing, WildCard: false}
	obj.Sites = append(obj.Sites, newSite)
	back, _ := json.MarshalIndent(obj, "", "  ")
	ioutil.WriteFile("/usr/Hosting/config.json", back, 0777)
	return nil
}

func deleteSiteFromJSON(wp wpdelete) error {
	// data, err := ioutil.ReadFile("/usr/Hosting/config.json")
	// if err != nil {
	// 	return echo.NewHTTPError(404, "Config file not found")
	// }

	// // json data
	// var obj Config

	// // unmarshall it
	// err = json.Unmarshal(data, &obj)
	// if err != nil {
	// 	return echo.NewHTTPError(400, "JSON data error")
	// }

	for i, site := range obj.Sites {
		if site.Name == wp.AppName {
			obj.Sites = RemoveIndex(obj.Sites, i)
		}
	}

	if len(obj.Sites) == 0 {
		obj.SSL = false
	}

	for _, site := range obj.Sites {
		if site.PrimaryDomain.SSL == true {
			obj.SSL = true
			break
		}
		obj.SSL = false
	}

	back, _ := json.MarshalIndent(obj, "", "  ")
	err := ioutil.WriteFile("/usr/Hosting/config.json", back, 0777)
	if err != nil {
		return echo.NewHTTPError(400, "Cannot write to config file")
	}
	return nil
}

func getSites(c echo.Context) error {

	return c.JSON(http.StatusOK, &obj.Sites)
}

func RemoveIndex(s []Site, index int) []Site {
	return append(s[:index], s[index+1:]...)
}

func addCert(wp wpcert) error {

	data, _ := ioutil.ReadFile("/usr/Hosting/config.json")

	// json data
	var obj Config

	// unmarshall it
	err := json.Unmarshal(data, &obj)
	if err != nil {
		return echo.NewHTTPError(400, "JSON data error")
	}

	for i, site := range obj.Sites {
		if wp.AppName == site.Name {
			if wp.Url == site.PrimaryDomain.Url {
				_, err := exec.Command("/bin/bash", "-c", fmt.Sprintf("service hosting stop; certbot certonly --standalone --dry-run -d %s", wp.Url)).Output()
				if err != nil {
					return echo.NewHTTPError(404, "Error with cert config")
				}
				_, err = exec.Command("/bin/bash", "-c", fmt.Sprintf("certbot certonly --standalone -d %s -m %s --agree-tos --cert-name %s", wp.Url, wp.Email, wp.Url)).Output()
				if err != nil {
					return echo.NewHTTPError(404, "Error with cert config after dry run")
				}
				obj.SSL = true
				exec.Command("/bin/bash", "-c", fmt.Sprintf("cat /etc/letsencrypt/live/%s/fullchain.pem /etc/letsencrypt/live/%s/privkey.pem > /opt/Hosting/certs/%s.pem", wp.Url, wp.Url, wp.Url))
				obj.Sites[i].PrimaryDomain.SSL = true
				back, _ := json.MarshalIndent(obj, "", "  ")
				err = ioutil.WriteFile("/usr/Hosting/config.json", back, 0777)
				if err != nil {
					return echo.NewHTTPError(400, "Cannot write to config file")
				}
				configNuster()
				exec.Command("/bin/bash", "-c", "service hosting start")
				return nil
			}
		}
	}

	return echo.NewHTTPError(404, "Domain not found with this app")
}

// define data structure
type Global struct {
	Datasize int `json:"dataSize"`
	Maxconn  int `json:"maxConnection"`
}
type Timeout struct {
	Connect int `json:"connect"`
	Client  int `json:"client"`
	Server  int `json:"server"`
}

type Default struct {
	Timeout Timeout `json:"timeout"`
}

type Site struct {
	Name          string   `json:"name"`
	PrimaryDomain Domain   `json:"primaryDomain"`
	AliasDomain   []Domain `json:"aliasDomain"`
	Cache         string   `json:"cache"`
	Exclude       []string `json:"exclude"`
}

type Config struct {
	Global  Global  `json:"global"`
	Default Default `json:"defaults"`
	Sites   []Site  `json:"sites"`
	SSL     bool    `json:"ssl"`
}

type Domain struct {
	Url       string `json:"url"`
	SubDomain bool   `json:"subDomain"`
	SSL       bool   `json:"ssl"`
	WildCard  bool   `json:"wildcard"`
	Routing   string `json:"routing"`
}
