package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
)

type githubRepo struct {
	Name        string `json:"name"`
	URL         string `json:"html_url"`
	Description string `json:"description"`
	Fork        bool   `json:"fork"`
}

type gogsRepo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Mirror      bool   `json:"mirror"`
}

type gogs struct {
	url        string
	token      string
	uid        int
	uidFetched bool
	client     *http.Client
}

var (
	appName    = "github2gogs"
	appVersion = "dev"
	buildTime  = "unknow"
)

func main() {
	ver := flag.Bool("version", false, "show version")
	token := flag.String("token", "", "access `token` info for gogs")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [flags] <github_username> <gogs_url>\n\n", appName)
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExample: %s -token 0123456789abcdef golang https://gogs.some.com\n", appName)
	}
	flag.Parse()
	if *ver {
		fmt.Println("version", appVersion)
		fmt.Println("build time", buildTime)
		os.Exit(1)
	}
	if flag.NArg() < 2 {
		flag.Usage()
		os.Exit(1)
	}
	err := run(flag.Arg(0), flag.Arg(1), *token)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
	}
}

func run(src, dst, token string) error {
	srcRepos, err := fetchGithubRepos(src)
	if err != nil {
		return err
	}
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	g := &gogs{dst, token, 0, false, &http.Client{Transport: tr}}
	dstRepos, err := g.fetchGogsRepos()
	if err != nil {
		return err
	}
	mRepos, conflict, already := filterRepos(srcRepos, dstRepos)
	fmt.Printf("%d repos need to migrate, %d conflict repos ignored, %d repos already exist\n", len(mRepos), conflict, already)
	for _, r := range mRepos {
		fmt.Printf("migrating %s\n", r.Name)
		if err := g.migrate(r); err != nil {
			return err
		}
	}
	return nil
}

func fetchGithubRepos(user string) ([]githubRepo, error) {
	srcRepos := []githubRepo{}
	idx := 1
	for {
		res, err := http.Get(fmt.Sprintf("https://api.github.com/users/%s/repos?page=%d&per_page=50", user, idx))
		if err != nil {
			return nil, err
		}
		if res.StatusCode != 200 {
			return nil, errors.New(res.Status)
		}
		b, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return nil, err
		}
		result := []githubRepo{}
		if err = json.Unmarshal(b, &result); err != nil {
			return nil, err
		}
		srcRepos = append(srcRepos, result...)
		if len(result) < 50 {
			break
		}
		idx++
	}
	return srcRepos, nil
}

func (g *gogs) fetchGogsRepos() ([]gogsRepo, error) {
	repos := []gogsRepo{}
	req, _ := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/user/repos", g.url), nil)
	g.handleAuth(req)
	res, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != 200 {
		return nil, errors.New(res.Status)
	}
	b, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(b, &repos); err != nil {
		return nil, err
	}
	return repos, nil
}

func (g *gogs) fetchUserInfo() error {
	if g.uidFetched {
		return nil
	}
	req, _ := http.NewRequest("GET", fmt.Sprintf("%s/api/v1/user", g.url), nil)
	g.handleAuth(req)
	res, err := g.client.Do(req)
	if err != nil {
		return err
	}
	if res.StatusCode != 200 {
		return errors.New(res.Status)
	}
	b, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}
	var obj struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(b, &obj); err != nil {
		return err
	}
	g.uid = obj.ID
	return nil
}

func (g *gogs) migrate(srcRepo githubRepo) error {
	if err := g.fetchUserInfo(); err != nil {
		return err
	}
	formMap := map[string]interface{}{}
	formMap["clone_addr"] = srcRepo.URL
	formMap["description"] = srcRepo.Description
	formMap["mirror"] = true
	formMap["repo_name"] = srcRepo.Name
	formMap["uid"] = g.uid
	formBytes, err := json.Marshal(formMap)
	if err != nil {
		return err
	}
	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/repos/migrate", g.url), bytes.NewBuffer(formBytes))
	req.Header.Set("Content-Type", "application/json")
	g.handleAuth(req)
	res, err := g.client.Do(req)
	if err != nil {
		return err
	}
	if res.StatusCode != 201 {
		return errors.New(res.Status)
	}
	return nil
}

func (g *gogs) handleAuth(req *http.Request) {
	if g.token == "" {
		return
	}
	req.Header.Set("Authorization", "token "+g.token)
}

func filterRepos(srcRepos []githubRepo, dstRepos []gogsRepo) (result []githubRepo, conflict int, alreadyMigrated int) {
	dstRepoMap := map[string]*gogsRepo{}
	for idx := range dstRepos {
		r := &dstRepos[idx]
		dstRepoMap[r.Name] = r
	}
	for _, repo := range srcRepos {
		dstRepo := dstRepoMap[repo.Name]
		if dstRepo != nil {
			if dstRepo.Mirror {
				alreadyMigrated++
			} else {
				conflict++
			}
		} else {
			result = append(result, repo)
		}
	}
	return
}
