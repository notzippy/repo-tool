package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"strconv"
	"sync"
	"time"
    "regexp"
)

const baseRepo = ".repo-tool"
const projectFile = ".repo-tool/projectRepo.json"

var (
	VERSION      = "0.4"
	relativePath = ""
	gitCmd       = "/usr/bin/git"
	hgCmd        = "/usr/bin/hg"
    ThereWasErrors = false
)

const (
	REPO_GIT="git"
	REPO_HG="hg"
)


type (
	Repo struct {
		Root           string // The root checkout
		Branch         string
        GroupList      []string // The groups of projects to be imported
		RemoteMap      map[string]*Remote  // Keyed by name
		ProjectMap     map[string]*Project // Keyed by name
		ProjectDefault *Project
	}
	Manifest struct {
		XMLName        xml.Name   `xml:"manifest"`
		ProjectList    []*Project `xml:"project"`
		RemoteList     []*Remote  `xml:"remote"`
		ProjectDefault *Project   `xml:"default"`
	}
	Project struct {
		//XMLName xml.Name `xml:"project"`
		Name               string `xml:"name,attr"`
		RemoteKey          string `xml:"remote,attr"`
		BranchRevision     string `xml:"-"`
		Revision           string `xml:"revision,attr"`
		Path               string `xml:"path,attr"`
		Review             string `xml:"review,attr"`
        GroupList        []string                        // Note groups in notdefault will not be downloaded
        Groups             string `xml:"groups,attr"`   // Note groups in notdefault will not be downloaded
        CloneDepth         string `xml:"clone-depth,attr"`
		RepoType           string `xml:"repotype,attr"` // The type of repository
		Url                string // The remote url
		PrimaryBareGitPath string // project-objects
		SecondaryGitPath   string // projects
		SourceGitPath      string // project git path as defined in the file
		SourcePath         string // project path as defined in the file
	}
	Remote struct {
		Name               string `xml:"name,attr"`
		Fetch              string `xml:"fetch,attr"`
		Review             string `xml:"review,attr"`
		Revision           string `xml:"revision,attr"`
		RepoType           string `xml:"repotype,attr"` // The type of repository
	}
)

func main() {
	if git, err := exec.LookPath("git"); err != nil {
		panic("Cannot find git command!")
	} else {
		gitCmd = git
	}
	runtime.GOMAXPROCS(runtime.NumCPU())

	if len(os.Args) < 2 {
		help()
		return
	}
	start := time.Now()
	fmt.Printf("Start time %s\n", start.Format(time.ANSIC))
	switch os.Args[1] {
	case "init":
		if err := repoInit(os.Args[2:]); err != nil {
			fmt.Println("Error init ", err)
            ThereWasErrors = true
		}
	case "sync":
		if project, err := fetchRepo(); err != nil {
			fmt.Println("Error read project ", err)
            ThereWasErrors = true
		} else if err := repoSync(project,os.Args[2:]); err != nil {
			fmt.Println("Error Sync ", err)
            ThereWasErrors = true
		}
	case "status":
		if err := repoStatus(); err != nil {
			fmt.Println("Error Status ", err)
		}
	default:
		fmt.Println("Invalid command " + os.Args[1])
		help()
	}
	end := time.Now()
	fmt.Printf("End time %s\n", end.Format(time.ANSIC))
	fmt.Printf("End time %2.4f minutes\n", end.Sub(start).Minutes())
    if ThereWasErrors {
        fmt.Println("There was errors detected, please check messages")
    }  else {
        fmt.Println("Success - no errors detected")
    }
}

// Initialize the project, recognized options are
// -u The URL  of the remote manifest repository (Required)
// -b The branch of the remote manifest repository (defaults to master)
// -g A comma delimited list of groups to download
func repoInit(osargs []string) (err error) {
	var url, branch, groups string
	mySet := flag.NewFlagSet("", flag.ExitOnError)
	mySet.StringVar(&url, "u", "", "a url")
	mySet.StringVar(&branch, "b", "", "a branch")
	mySet.StringVar(&groups, "g", "", "a group")
	mySet.Parse(osargs)
	if len(url) == 0 {
		return fmt.Errorf("Url required")
	}

	var args []string
	if len(branch) > 0 {
		args = append(args, "-b", branch)
	}

    projectRepo := Repo{
		Root:       url,
		Branch:     branch,
        GroupList:  strings.Split(groups,","),
		ProjectMap: make(map[string]*Project),
		RemoteMap:  make(map[string]*Remote),
	}

    if project, err := fetchRepo(); err == nil {
        project.Root = url
        project.Branch = branch
        project.GroupList = strings.Split(groups,",")
        projectRepo = *project
    } else {
        relativePath=""
    }


	i := strings.LastIndex(url, "/")
	if i > 0 {
		projectRepo.Root = url[:i]
	}

    // Check to see if the manifest file exists, if so we need to update, not clone
    if _,e:=os.Stat(filepath.Join(relativePath,baseRepo, "manifests"));e==nil {
        if syncManifest(&projectRepo)!=nil {
            fmt.Printf("You may want to consider removing your manifest folder");
            return err
        }
    } else {
        if _, err = gitClone(url,
            filepath.Join(relativePath,baseRepo, "manifests"),
            "",
            args); err != nil {
                return
        }
    }


	// We need to write it because the next step we resync
	if err = writeProject(&projectRepo); err != nil {
		return
	}
	return repoSync(&projectRepo,osargs)
}

// Read the manifest in the repo-tool folder
// Currently reads only one
func refreshProjectRepo(projectRepo *Repo) (err error) {

	if data, err := ioutil.ReadFile(filepath.Join(relativePath, baseRepo, "manifests", "default.xml")); err != nil {
		return err
	} else {
		if err = projectRepo.Parse(filepath.Join(relativePath, baseRepo, "manifests", "default.xml"), data); err != nil {
			return err
		}

	}

	return
}
func writeProject(projectRepo *Repo) (err error) {
	// Dump the project repo to the baseRepo folder
	// as a json file
	if b, e := json.MarshalIndent(projectRepo, "", " "); e != nil {
		return e
	} else {
		err = ioutil.WriteFile(filepath.Join(relativePath, projectFile), b, os.ModePerm)
	}
	return
}
func syncManifest(repo *Repo) (err error) {
	if _, err = gitSync(
		filepath.Join(relativePath, baseRepo, "manifests"),
		"origin",
		repo.Branch, nil); err != nil {

		return fmt.Errorf("Failed to sync manifests %v", err)
	}
    return err
}
func repoSync(existingRepo *Repo,osargs []string) (err error) {
    multitasksLimit := 1;
    for _,a:=range osargs {
        if a[:2]=="-j" {
            i,_ :=  strconv.ParseInt(a[2:],10,8)
            multitasksLimit = int(i)
        }
    }
    fmt.Println("Limit is ", multitasksLimit)
	if err = syncManifest(existingRepo);err!=nil {
        return err
	}
	// Sync the manifest first
	projectRepo := Repo{
		Root:       existingRepo.Root,
		Branch:     existingRepo.Branch,
        GroupList:  existingRepo.GroupList,
		ProjectMap: make(map[string]*Project),
		RemoteMap:  make(map[string]*Remote),
	}
	if err=refreshProjectRepo(&projectRepo);err!=nil {
		return
	}

    // Make sure the passed in manifest parameters are persisted after reload of json
    projectRepo.Root = existingRepo.Root
    projectRepo.Branch = existingRepo.Branch
    projectRepo.GroupList = existingRepo.GroupList

    // fmt.Printf("Current group %#v %d\n",projectRepo.GroupList, len(existingRepo.ProjectMap))
	if len(existingRepo.ProjectMap) > 0 {
		// Check to see if the projects have been moved, or paths have changed
		for _, p := range existingRepo.ProjectMap {
			found := false

			for _, pnew := range projectRepo.ProjectMap {
				if pnew.Url == p.Url {

					if pnew.Path != p.Path {
						destination := filepath.Join(relativePath, pnew.Path)
						base := filepath.Base(destination)
						if err = os.MkdirAll(destination[:len(destination)-len(base)], os.ModePerm);err!=nil {
							return
						}
                        if _, err := os.Stat(filepath.Join(relativePath, p.Path)); err==nil {
                            if err = os.Rename(filepath.Join(relativePath, p.Path), destination); err != nil {
                                return err
                            }
                        }
					}
					found = true
					break
				}
			}

            if found && !p.InGroupList(projectRepo.GroupList) {
                // Force the removal of the project to the trash
                found = false
            }
			if !found {
                // Check to see if project exists in path before removing
                if _,e:= os.Stat(filepath.Join(relativePath,p.Path));e==nil {
                    // Move the project into the trash, if not in new list
                    destination := filepath.Join(relativePath, ".trash", p.Path)
                    base := filepath.Base(destination)
                    os.MkdirAll(destination[:len(destination)-len(base)], os.ModePerm)
                    fmt.Println("Moving project", p.Name, "Into .trash")
                    if _, err := os.Stat(filepath.Join(relativePath, p.Path)); err==nil {
                        if err = os.Rename(filepath.Join(relativePath, p.Path), destination); err != nil {
                            return  err
                        }
                    }
                }
			}
		}
	}
	writeProject(&projectRepo)

	// For each item in the repo checkout the source tree
	var wg sync.WaitGroup
    var mutex sync.Mutex
    counter:=0
	for _, project := range projectRepo.ProjectMap {
        if project.InGroupList(projectRepo.GroupList) {
            wg.Add(1)
            mutex.Lock()
            counter++
            mutex.Unlock()
            go func(mproject *Project) {
                defer wg.Done()
                defer func() {
                    mutex.Lock()
                    counter--
                    mutex.Unlock()
                }()

                if e := mproject.checkout(); e != nil {
                    ThereWasErrors = true
                    fmt.Println("Error sync ", mproject.Name, err)
                }

            }(project)
            if counter>=multitasksLimit {
                wg.Wait()
            }
        } else {
            fmt.Println("Skipping project",project.Name,projectRepo.GroupList)
        }
	}
	wg.Wait()

	return
}
func repoStatus() (err error) {
	// read in the project repo
	var projectRepo *Repo
	if projectRepo, err = fetchRepo(); err != nil {
		return
	}

	// For each item in the repo checkout the source tree
	var wg sync.WaitGroup
	for _, project := range projectRepo.ProjectMap {
		wg.Add(1)
		go func(mproject *Project) {
			defer wg.Done()
			if e := mproject.status(); e != nil {
				fmt.Println("Error status ", mproject.Name, err)
			}

		}(project)
		wg.Wait()
	}

	return
}

// Checkout project, clones then does a checkout -b revision
func (p *Project) checkout() (err error) {
	fmt.Println("Checkout ", p.RepoType)
	switch p.RepoType {
	case REPO_HG : {
		if _, e := os.Stat(filepath.Join(filepath.Join(relativePath, p.SourcePath), ".hg")); e != nil {
			// Create a hg clone
			args := []string{
				"--branch", p.BranchRevision,

			}
			if out, e := hgClone(p.Url, filepath.Join(relativePath, p.SourcePath), args); e != nil {
				fmt.Printf("Project %s Clone (CLONE) Error \n%s\n%v\n\n", p.Name, out, e)
				return e
			} else {
				fmt.Printf("Project %s Clone (END) \n%s\n\n", p.Name, out)
			}

		} else {
			// Update HG and source
			out, e := hgSync(filepath.Join(relativePath, p.SourcePath), p.RemoteKey, p.Revision, nil)
			if e != nil {
				ThereWasErrors = true
				fmt.Printf("Project %s Synchronize (PULL error) \n%s\n%v\n\n", p.Name, out, e)
				return e
			}
			fmt.Printf("Project %s Synchronize (PULL) \n%s\n\n", p.Name, out)

		}

	}
	case REPO_GIT :
		if _, e := os.Stat(filepath.Join(filepath.Join(relativePath, p.SourcePath), ".git")); e != nil {
			args := []string{
				"--branch", p.BranchRevision,
				"-o", p.RemoteKey,
			}
			if p.CloneDepth != "" {
				args = append(args, "--depth", p.CloneDepth)
			}

			if out, e := gitClone(p.Url, filepath.Join(relativePath, p.SourcePath), "", args); e != nil {
				fmt.Printf("Project %s Clone (CLONE) Error \n%s\n%v\n\n", p.Name, out, e)
				return e
			} else {
				fmt.Printf("Project %s Clone (END) \n%s\n\n", p.Name, out)
			}


		} else {
			out, e := gitSync(filepath.Join(relativePath, p.SourcePath), p.RemoteKey, p.Revision, nil)
			if e != nil {
				ThereWasErrors = true
				fmt.Printf("Project %s Synchronize (PULL error) \n%s\n%v\n\n", p.Name, out, e)
				return e
			}
			fmt.Printf("Project %s Synchronize (PULL) \n%s\n\n", p.Name, out)
		}
	}
	return
}

// Returns the status of the project
func (p *Project) status() (err error)  {
	switch p.RepoType {
	case REPO_HG:
		if _, e := os.Stat(filepath.Join(filepath.Join(relativePath, p.SourcePath), ".hg")); e == nil {
			status, _ := hgStatus(filepath.Join(relativePath, p.SourcePath), nil)
			if !strings.Contains(status,"clean") {
				fmt.Sprintf("** Project %s Not Clean status **\n%s\n** END Project %s status (path %s) **\n\n", p.Name, status, p.Name, p.Path)
			}

		} else {
			fmt.Printf("Project %s missing\n", p.Name)
		}
	case REPO_GIT:
		if _, e := os.Stat(filepath.Join(filepath.Join(relativePath, p.SourcePath), ".git")); e == nil {
			status, _ := gitStatus(filepath.Join(relativePath, p.SourcePath), nil)
			if !strings.Contains(status,"clean") {
				fmt.Printf("** Project %s Not Clean status **\n%s\n** END Project %s status (path %s) **\n\n", p.Name, status, p.Name, p.Path)
			}

		} else {
			fmt.Printf("Project %s missing\n", p.Name)
		}
	}
	return
}

// Parses the data in the manifest file
//
func (r *Repo) Parse(id string, data []byte) (err error) {
	var decoded Manifest
	if err = xml.Unmarshal(data, &decoded); err == nil {
		// data is decoded, read it into the repo
		for _, remote := range decoded.RemoteList {
			if _, ok := r.RemoteMap[remote.Name]; ok {
				return fmt.Errorf("Remote name duplicated in %s", id)
			}
			r.RemoteMap[remote.Name] = remote
		}

		if r.ProjectDefault == nil {
			if decoded.ProjectDefault != nil {
				r.ProjectDefault = decoded.ProjectDefault
			}
		} else if decoded.ProjectDefault != nil {
			return fmt.Errorf("Second project default found in %s\n", id)

		}
		for _, project := range decoded.ProjectList {
            // Initialize projects based on combination of default and remote
			if r.ProjectDefault != nil {
				if len(project.RemoteKey) == 0 {
					project.RemoteKey = r.ProjectDefault.RemoteKey
				}
				if len(project.Review) == 0 {
					project.Review = r.ProjectDefault.Review
				}
			}
			if len(project.Name) == 0 {
				return fmt.Errorf("Project missing name in %s\n", id)
			}
			if len(project.Path) == 0 {
				return fmt.Errorf("Project missing name in %s\n", id)
			}
			if _, ok := r.ProjectMap[project.Name]; ok {
				return fmt.Errorf("Duplicate project %s %s\n", project.Name, id)
			}
			url := project.Name
			if repo, ok := r.RemoteMap[project.RemoteKey]; !ok {
				return fmt.Errorf("Remote not found %s\n", project.RemoteKey)
			} else {
				if len(project.Revision) == 0 {
					if repo.Revision!="" {
						project.Revision = repo.Revision
					} else {
						project.Revision = r.ProjectDefault.Revision
					}
				}

				if strings.HasPrefix("..", repo.Fetch) {
					// This is a relative url
					if len(repo.Fetch) > 2 {
						url = joinURL(r.Root , repo.Fetch[2:] , project.Name)
					} else {
						url = joinURL(r.Root ,project.Name)
					}
				} else {
					url = joinURL(repo.Fetch , project.Name)
				}

				if repo.RepoType == "" {
					project.RepoType = REPO_GIT
				} else {
					project.RepoType = repo.RepoType
				}
			}
            // Initialize calculate values
			project.Url = url
			project.PrimaryBareGitPath = filepath.Join(filepath.Join(baseRepo, "project-objects"), project.Name+".git")
			project.SecondaryGitPath = filepath.Join(filepath.Join(baseRepo, "projects"), project.Path, ".git")
			project.SourceGitPath = filepath.Join(project.Path, ".git")
			project.SourcePath = project.Path
			r.ProjectMap[project.Name] = project
            BranchRevision:=project.Revision
            if strings.HasPrefix(BranchRevision,"refs/heads/") {
                BranchRevision =BranchRevision[len("refs/heads/"):]
            }
            if strings.HasPrefix(BranchRevision,"refs/tags/") {
                BranchRevision =BranchRevision[len("refs/tags/"):]
            }
            project.BranchRevision=BranchRevision
            project.GroupList=regexp.MustCompile(`[\ ,\,]`).Split(project.Groups,-1)
		}
	}

	return
}
func joinURL(parts ...string) (string) {
    for i,p:=range parts {
        parts[i]=strings.TrimSuffix(p,"/")
    }
    return strings.Join(parts,"/")
}

// Returns true if project is in group list
// if groupList to be checked is empty then
// result is true unless this project contains a
// group call `notdefault`
func (p *Project) InGroupList(groupList[]string)(bool) {
    if len(groupList)==0 {
        // Exclude default Projects
        return !p.HasGroup("notdefault")
    }

    for _,g:=range groupList {
        if g=="" && !p.HasGroup("notdefault") {
            return true
        } else if p.HasGroup(g) {
            return true
        }
    }

    return false
}

// Return true if group is in this projects group list
func (p *Project) HasGroup(group string)(bool) {
    if len(p.GroupList)==0 {
        return group!="notdefault"
    }
    for _,g:= range p.GroupList {
        if g==group {
            return true
        }
    }

    return false
}

// Called to checkout a project, creates the neccessary path to target folder
func gitClone(source, target, gitfolder string, args []string) (out string, err error) {
	// fmt.Printf("Checking out " + source + " to " + target +" %#v\n", args)
	if len(gitfolder) > 0 {
		// ensure absolute path is specified
		a, _ := filepath.Abs(gitfolder)
		target, _ = filepath.Abs(target)
		args = append(args, "--separate-git-dir", a)
		// Ensure parent folder exists todo optimize
		os.MkdirAll(gitfolder, os.ModePerm)
		os.Remove(gitfolder)
	} else {

	}
	cmd, stdout, stderr := Git("clone", append(args, source, target)...)
	if err = cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("Error running clone command %s", stderr.String())
	}

	if len(gitfolder) > 0 {
		// Remove the helpful .git file from target and create a symlink to the source
		os.Remove(filepath.Join(target, ".git"))
		os.Symlink(gitfolder, filepath.Join(target, ".git"))
	}
	return stdout.String(), nil
}
// Called to checkout a project, creates the necessary path to target folder
func hgClone(source, target string, args []string) (out string, err error) {
	// fmt.Printf("Checking out " + source + " to " + target +" %#v\n", args)
	cmd, stdout, stderr := Hg("clone", append(args, source, target)...)
	if err = cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("Error running HG clone command %s", stderr.String())
	}

	return stdout.String(), nil
}

// Called to synchronize a project
func gitSync(source, remote, revision string, args []string) (out string, err error) {

	cmd, stdout, stderr := Git("-C", append([]string{source, "pull", remote, revision}, args...)...)

	if err = cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("Error running fetch command %s", stderr.String())
	}
	/*
	cmd, stdout, stderr := Git("-C", append([]string{source, "fetch", remote, revision}, args...)...)

	if err = cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("Error running fetch command %s", stderr.String())
	}

	//cmd, stdout, stderr := Git("-C", append([]string{source, "status"}, args...)...)
	checkoutArgs := []string{source,"checkout"}
	if revision!="" {
		checkoutArgs = []string{source,"checkout",revision}
	}

	cmd, stdout, stderr = Git("-C", append(checkoutArgs, args...)...)
	if err = cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("Error running checkout command %s", stderr.String())
	}

	cmd, stdout, stderr = Git("-C", append([]string{source, "pull", }, args...)...)

	if err = cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("Error running pull command %s", stderr.String())
	}
	*/
	return stdout.String(), nil
}
// Called to synchronize a project
func hgSync(source, remote, revision string, args []string) (out string, err error) {
	{
		cmd, stdout, stderr := Hg("-R", append([]string{source, "pull"}, args...)...)

		if err = cmd.Run(); err != nil {
			return stdout.String(), fmt.Errorf("Error running clone command %s", stderr.String())
		}
	}
	cmd, stdout, stderr := Hg("-R", append([]string{source, "checkout", revision}, args...)...)

	if err = cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("Error running clone command %s", stderr.String())
	}

	return stdout.String(), nil
}

// Called to fetch a project status
func gitStatus(source string, args []string) (respons string, err error) {

	cmd, stdout, stderr := Git("-C", append([]string{source, "status"}, args...)...)

	if err = cmd.Run(); err != nil {
		return "", fmt.Errorf("Error running clone command %s\n", stderr.String())
	}

	return stdout.String(), nil
}
// Called to fetch a project status
func hgStatus(source string, args []string) (respons string, err error) {

	cmd, stdout, stderr := Hg("-R", append([]string{source, "status"}, args...)...)

	if err = cmd.Run(); err != nil {
		return "", fmt.Errorf("Error running clone command %s\n", stderr.String())
	}

	return stdout.String(), nil
}

// Called to perform all git commands
func Git(cmd string, args ...string) (res *exec.Cmd, stdout, stderr *bytes.Buffer) {
	cmdArgs := make([]string, 1)
	cmdArgs[0] = cmd
	cmdArgs = append(cmdArgs, args...)

	res = exec.Command(gitCmd, cmdArgs...)
	stdout, stderr = new(bytes.Buffer), new(bytes.Buffer)
	stdout.WriteString(fmt.Sprintf("git %s %v\n", gitCmd, cmdArgs))
	res.Stdout, res.Stderr = os.Stdout, os.Stderr // io.MultiWriter(stdout,os.Stdout), io.MultiWriter(stderr,os.Stderr)
	go io.Copy(os.Stdout, stdout)
	go io.Copy(os.Stderr, stderr)
	return
}

// Called to perform all hg commands
func Hg(cmd string, args ...string) (res *exec.Cmd, stdout, stderr *bytes.Buffer) {
	cmdArgs := make([]string, 1)
	cmdArgs[0] = cmd
	cmdArgs = append(cmdArgs, args...)

	res = exec.Command(hgCmd, cmdArgs...)
	stdout, stderr = new(bytes.Buffer), new(bytes.Buffer)
	stdout.WriteString(fmt.Sprintf("Running *** hg %s %v\n", hgCmd, cmdArgs))
	res.Stdout, res.Stderr = os.Stdout, os.Stderr // io.MultiWriter(stdout,os.Stdout), io.MultiWriter(stderr,os.Stderr)
	go io.Copy(os.Stdout, stdout)
	go io.Copy(os.Stderr, stderr)
	return
}

// Called to fetch the repo project object that was saved
// initially int the projectFile (".repo-tool/projectRepo.json")
// This function uses the relativePath to resolve the project File
func fetchRepo() (therepo *Repo,err error) {
	repo := new(Repo)

	oldPath := ""

    // Check the existance of the projectFile using the relative path
	_, e := os.Stat(filepath.Join(relativePath, projectFile))
    fmt.Println("Checked",filepath.Join(relativePath, projectFile),e)

    // If not found attempt to go up directory tree
	for e != nil {
		newPath, _ := filepath.Abs(filepath.Join(relativePath, projectFile))
		if newPath == oldPath {
			return nil, fmt.Errorf("Unable to locate relative repo path")
		}
		oldPath = newPath
		relativePath = filepath.Join(relativePath, "..")
		_, e = os.Stat(filepath.Join(relativePath, projectFile))
	}

    // If able to read file, call the parse and return the repo
	if b, err := ioutil.ReadFile(filepath.Join(relativePath, projectFile)); err != nil {
        relativePath=""
		return nil, err
	} else  if e := json.Unmarshal(b, repo); e != nil {
        return repo, e
    }
	return repo,nil
}

// Small help function
func help() {
	fmt.Printf("You need help (%s), try repo-tool init, repo-tool status or repo-tool sync.ie repo-tool init -u <manifest git url> for the manifest \n",VERSION)
}

// Assuming the base of the path is baseRepo
// the relative path is the number of "/" characters
// to the current folder, this will return a path which can be appended
// a path to
func relativeToRepo(path string) string {
	relativepath := "."
	for x := 0; x < strings.Count(path, "/")+1; x++ {
		relativepath = filepath.Join("..", relativepath)
	}
	return relativepath
}
