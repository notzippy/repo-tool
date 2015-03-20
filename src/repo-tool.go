package main

import ("fmt"
	"flag"
	"os"
	"path/filepath"
	"io/ioutil"
	"encoding/xml"
	"strings"
	"encoding/json"
	"os/exec"
	"bytes"
	"io"
	"sync"
	"runtime"
	"time"
)
const baseRepo = ".repo-tool"
const projectFile = ".repo-tool/projectRepo.json"
var (
	relativePath = ""
	 gitCmd = "/usr/bin/git"
//	reposymlinkFile = []string{"description", "info"}
//	reposymlinkDir = []string{"hooks","objects","rr-cache","svn"}
//	reposymlinkRefFile = []string{"description", "info","config","packed-refs","shallow"}
//	reposymlinkRefDir = []string{"hooks","objects","rr-cache","svn","logs","refs"}

)
type (
	Repo struct {
		Root string // The root checkout
		Branch string
		RemoteMap map[string]*Remote    // Keyed by name
		ProjectMap map[string]*Project  // Keyed by name
		ProjectDefault *Project  // Keyed by name
	}
	Manifest struct {
		XMLName xml.Name `xml:"manifest"`
		ProjectList []*Project `xml:"project"`
		RemoteList []*Remote   `xml:"remote"`
		ProjectDefault *Project  `xml:"default"`
	}
	Project struct {
		//XMLName xml.Name `xml:"project"`
		Name string `xml:"name,attr"`
		RemoteKey string `xml:"remote,attr"`
		Revision string `xml:"revision,attr"`
		Path string `xml:"path,attr"`
		Review string `xml:"review,attr"`
		Url string 							// The remote url
		PrimaryBareGitPath string 			// project-objects
		SecondaryGitPath string 			// projects
		SourceGitPath string 				// project git path as defined in the file
		SourcePath string 					// project path as defined in the file

	}
	Remote struct {
		Name string  `xml:"name,attr"`
		Fetch string `xml:"fetch,attr"`
		Review string `xml:"review,attr"`
	}
)
func main() {
	if git, err := exec.LookPath("git"); err != nil {
		panic("Cannot find git command!")
	} else {
		gitCmd = git
	}
	runtime.GOMAXPROCS(runtime.NumCPU())

	if len(os.Args)<2 {
		help()
		return
	}
	start:=time.Now()
	fmt.Printf("Start time %s\n",start.Format(time.ANSIC))
	switch os.Args[1] {
		case "init"   :
			if err:=repoInit();err!=nil {
				fmt.Println("Error init ", err)
			}
	case "sync"   :
			if project, err:=fetchRepoProject();err!=nil {
				fmt.Println("Error read project ", err)
			} else if err:=repoSync(project);err!=nil {
				fmt.Println("Error Sync ", err)
			}
		case "status"   :
			if err:=repoStatus();err!=nil {
				fmt.Println("Error Status ", err)
			}
	default :
		fmt.Println("Invalid command " + os.Args[1])
		help()
	}
	end:=time.Now()
	fmt.Printf("End time %s\n",end.Format(time.ANSIC))
	fmt.Printf("End time %2.4f minutes\n",end.Sub(start).Minutes())
}
func repoInit() (err error) {
	var url,branch string
	mySet := flag.NewFlagSet("",flag.ExitOnError)
	mySet.StringVar(&url,"u","","a url")
	mySet.StringVar(&branch,"b","","a branch")
	mySet.Parse(os.Args[2:])
	if len(url)==0 {
		fmt.Println("Url required")
		return
	}

	os.RemoveAll(baseRepo)

	var args  []string
	if len(branch)>0 {
		args = append(args,"-b",branch)
	}
	if _, err = gitCheckout(url,
		filepath.Join(baseRepo,"manifests"),
		"",
		args);err!=nil {
		return;
	}

	relativeToRepo(filepath.Join(baseRepo,"manifest.xml"))
	os.Symlink(filepath.Join(relativeToRepo(filepath.Join(baseRepo,"manifest.xml")),baseRepo,"manifests","default.xml"),filepath.Join(baseRepo,"manifest.xml"))

	// Read in the xml file, since the repo isnt a simple file read it into a map first
	projectRepo := Repo{
		Root: url,
		Branch:branch,
		ProjectMap:make(map[string]*Project),
		RemoteMap:make(map[string]*Remote),
	}
	i :=strings.LastIndex(url,"/")
	if i>0 {
		projectRepo.Root=url[:i]
	}
	// We need to write it because the next step we resync
	if err=writeProject(&projectRepo);err!=nil {
		return
	}
	return repoSync(&projectRepo)
}

// Read the manifest in the repo-tool folder
// Currently reads only one
func refreshProjectRepo(projectRepo *Repo) (err error) {

	if data,err:=ioutil.ReadFile(filepath.Join(baseRepo,"manifests","default.xml"));err!=nil {
		return err
	}   else {
		if  err = projectRepo.Parse(filepath.Join(baseRepo,"manifests","default.xml"),data);err!=nil {
			return err
		}

	}

	return
}
func writeProject(projectRepo *Repo) (err error){
	// Dump the project repo to the baseRepo folder
	// as a json file
	if b,e:=json.MarshalIndent(projectRepo,""," ");e!=nil {
		return e;
	} else {
		err= ioutil.WriteFile(filepath.Join(relativePath, projectFile),b,os.ModePerm)
	}
	return
}

func repoSync(existingRepo *Repo) (err error){
	// Sync the manifest first
	if _, err = gitSync(
		filepath.Join(relativePath,baseRepo,"manifests"),
		"origin",
		existingRepo.Branch,nil);err!=nil {
		fmt.Println("Failed to sync manifests" ,err)
		return;
	}
	projectRepo := Repo{
		Root: existingRepo.Root,
		Branch:existingRepo.Branch,
		ProjectMap:make(map[string]*Project),
		RemoteMap:make(map[string]*Remote),
	}
	refreshProjectRepo(&projectRepo)
	if len(existingRepo.ProjectMap)>0 {
		// Check to see if the projects have been moved, or paths have changed
		for _,p := range existingRepo.ProjectMap {
			found:=false
			for _,pnew := range projectRepo.ProjectMap {
				if pnew.Url==p.Url {
					if pnew.Path!=p.Path {
						destination := filepath.Join(relativePath,pnew.Path)
						base:= filepath.Base(destination)
						os.MkdirAll(destination[:len(destination)-len(base)],os.ModePerm)
						if err=os.Rename(filepath.Join(relativePath,p.Path),destination);err!=nil {
							return
						}
					}
					found = true
					break
				}
			}
			if !found {
				// Move the project into the trash, if not in new list
				destination := filepath.Join(relativePath,".trash",p.Path)
				base:= filepath.Base(destination)
				os.MkdirAll(destination[:len(destination)-len(base)],os.ModePerm)
				fmt.Println("Moving project",p.Name,"Into .trash")
				if err=os.Rename(filepath.Join(relativePath,p.Path),destination);err!=nil {
					return
				}
			}
		}
	}
	writeProject(&projectRepo)

	// For each item in the repo checkout the source tree
	var wg sync.WaitGroup
	for _,project:=range projectRepo.ProjectMap{
		wg.Add(1)
		go func(mproject *Project) {
			defer wg.Done()
			if e:= mproject.checkout();e!=nil {
				fmt.Println("Error sync ",mproject.Name, err)
			}

		}(project)
	}
	wg.Wait()

	return
}
func repoStatus() (err error){
	// read in the project repo
	var projectRepo *Repo
	if projectRepo,err=fetchRepoProject();err!=nil {
		return
	}

	// For each item in the repo checkout the source tree
	var wg sync.WaitGroup
	for _,project:=range projectRepo.ProjectMap{
		wg.Add(1)
		go func(mproject *Project) {
			defer wg.Done()
			if e:= mproject.status();e!=nil {
				fmt.Println("Error status ",mproject.Name, err)
			}

		}(project)
	}
	wg.Wait()

	return
}
func (p *Project) checkout() (err error) {
	if _,e:=os.Stat(filepath.Join(filepath.Join(relativePath,p.SourcePath),".git"));e==nil {
		out,e := gitSync(filepath.Join(relativePath,p.SourcePath),p.RemoteKey,p.Revision,nil)
		fmt.Printf("Project %s Synchronize (PULL) \n%s\n%v\n\n",p.Name,out,e)
	} else {
		out,e:=gitCheckout(p.Url,filepath.Join(relativePath,p.SourcePath),"",nil)
		cmd,_,_:=Git("-C",filepath.Join(relativePath,p.SourcePath),"remote","rename","origin",p.RemoteKey)
		e = cmd.Run()
		fmt.Printf("Project %s Clone (CLONE) \n%s\n%v\n\n",p.Name,out,e)
	}
	return;
}
func (p *Project) status() (err error) {
	if _,e:=os.Stat(filepath.Join(filepath.Join(relativePath,p.SourcePath),".git"));e==nil {
		status,_ := gitStatus(filepath.Join(relativePath,p.SourcePath),nil)
		fmt.Printf("** Project %s status **\n%s\n** END Project %s status (path %s) **\n\n",p.Name,status,p.Name,p.Path)

	} else {
		fmt.Printf("Project %s missing\n",p.Name)
	}
	return;
}


// Parses the data
func (r *Repo) Parse(id string,data []byte) (err error) {
	var decoded  Manifest
	if err = xml.Unmarshal(data,&decoded);err==nil {
		// data is decoded, read it into the repo
		for _,remote := range decoded.RemoteList {
			if _,ok:=r.RemoteMap[remote.Name];ok {
				return fmt.Errorf("Remote name duplicated in %s",id)
			}
			r.RemoteMap[remote.Name]=remote
		}
		fmt.Printf("Remotes %#v", r.RemoteMap)
		if r.ProjectDefault==nil {
			if decoded.ProjectDefault!=nil {
				r.ProjectDefault  = decoded.ProjectDefault
			}
		} else if decoded.ProjectDefault!=nil {
			return fmt.Errorf("Second project default found in %s\n",id)

		}
		for _, project := range decoded.ProjectList {
			if r.ProjectDefault!=nil {
					if len(project.Revision)==0 {
						project.Revision = r.ProjectDefault.Revision
					}
					if len(project.RemoteKey)==0 {
						project.RemoteKey = r.ProjectDefault.RemoteKey
					}
				if len(project.Review)==0 {
					project.Review=r.ProjectDefault.Review
				}
			}
			if len(project.Name)==0 {
				return fmt.Errorf("Project missing name in %s\n",id)
			}
			if len(project.Path)==0 {
				return fmt.Errorf("Project missing name in %s\n",id)
			}
			if _,ok:= r.ProjectMap[project.Name];ok {
				return fmt.Errorf("Duplicate project %s %s\n",project.Name,id)
			}
			url := project.Name
			if repo,ok := r.RemoteMap[project.RemoteKey];!ok {
				return fmt.Errorf("Remote not found %s\n",project.RemoteKey)
			} else {
				if strings.HasPrefix("..",repo.Fetch) {
					// This is a relative url
					if len(repo.Fetch)>2 {
						url = r.Root+"/"+repo.Fetch[2:]+"/"+project.Name
					} else {
						url = r.Root+"/"+project.Name
					}
				} else {
					url =  repo.Fetch + "/" + project.Name
				}
			}
			project.Url=url
			project.PrimaryBareGitPath = filepath.Join(filepath.Join(baseRepo,"project-objects") , project.Name + ".git")
			project.SecondaryGitPath = filepath.Join(filepath.Join(baseRepo,"projects") , project.Path,".git")
			project.SourceGitPath = filepath.Join(project.Path,".git")
			project.SourcePath = project.Path
			r.ProjectMap[project.Name]=project
		}
	}

	return
}

func gitCheckout(source, target,gitfolder string, args []string) (out string, err error) {
	// fmt.Printf("Checking out " + source + " to " + target +" %#v\n", args)
	if len(gitfolder)>0 {
		// ensure absolute path is specified
		a,_ := filepath.Abs(gitfolder)
		target,_=filepath.Abs(target)
		args = append(args,"--separate-git-dir",a)
		// Ensure parent folder exists todo optimize
		os.MkdirAll(gitfolder,os.ModePerm)
		os.Remove(gitfolder)
	} else {

	}
	cmd, stdout, stderr := Git("clone", append(args, source, target)...)
	if err = cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("Error running clone command %s",stderr.String())
	}

	if len(gitfolder)>0 {
		// Remove the helpful .git file from target and create a symlink to the source
		os.Remove(filepath.Join(target,".git"))
		os.Symlink(gitfolder,filepath.Join(target,".git"))
	}
	return stdout.String(), nil
}
func gitSync(source,remote,revision string, args []string) (out string, err error) {


	cmd, stdout, stderr := Git("-C", append([]string{source,"pull", remote, revision},args...)...)
	//cmd.Stdout = os.Stdout
	//cmd.Stderr = os.Stderr
	if err = cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("Error running clone command %s",stderr.String())
	}

	return stdout.String(), nil
}
func gitStatus(source string, args []string) (respons string, err error) {

	cmd, stdout, stderr := Git("-C", append([]string{source,"status"},args...)...)

	//cmd.Stdout = os.Stdout
	//cmd.Stderr = os.Stderr
	if err = cmd.Run(); err != nil {

		return "", fmt.Errorf("Error running clone command %s\n",stderr.String())
	}

	return stdout.String(), nil
}

//func fetch(source, target string,args []string) ( err error) {
//	/*
//	cmd, _, stderr := Git("fetch", append([]string{"-C", target},args...)...)
//	cmd.Stdout = os.Stdout
//	cmd.Stderr = os.Stderr
//	if err = cmd.Run(); err != nil {
//		return errors.New(stderr.String())
//	}
//	*/
//	return GitCmdCall("fetch",target,"",args)
//}
//func processGitCommandList(list []GitCmd)(err error) {
//	for _,r:=range list {
//		if err=GitCmdCall(r.Command,r.Path,r.WorkingDir,r.Args);err!=nil {
//			return
//		}
//	}
//	return
//}
//func GitCmdCall(command,source, workingDir string,args []string) ( err error) {
//	cmd, _, stderr := Git(command, args...)
//	if len(source)>0 {
//		cmd.Env = append(cmd.Env, "GIT_DIR="+source)
//	}
//	cmd.Stdout = os.Stdout
//	cmd.Stderr = os.Stderr
//	if len(workingDir)>0 {
//		wd,_:=os.Getwd()
//		cmd.Dir = filepath.Join(wd, workingDir)
//	}
//	if err = cmd.Run(); err != nil {
//		fmt.Println("Error in run",err)
//		return errors.New(stderr.String())
//	}
//	return
//}
func Git(cmd string, args ...string) (res *exec.Cmd, stdout, stderr *bytes.Buffer) {
	cmdArgs := make([]string, 1)
	cmdArgs[0] = cmd
	cmdArgs = append(cmdArgs, args...)

	res = exec.Command(gitCmd, cmdArgs...)
	stdout, stderr = new(bytes.Buffer), new(bytes.Buffer)
	stdout.WriteString(fmt.Sprintf("git %s %v\n",gitCmd, cmdArgs))
	res.Stdout, res.Stderr = os.Stdout,os.Stderr // io.MultiWriter(stdout,os.Stdout), io.MultiWriter(stderr,os.Stderr)
	go io.Copy(os.Stdout,stdout)
	go io.Copy(os.Stderr,stderr)
	return
}

func fetchRepoProject() (project *Repo, err error ) {
	project = new(Repo)

	oldPath := ""
	_,e:=os.Stat(filepath.Join(relativePath,projectFile))
	for e!=nil {
		newPath,_:=filepath.Abs(filepath.Join(relativePath,projectFile))
		if newPath==oldPath {
			return nil, fmt.Errorf("Unable to locate relative repo path")
		}
		oldPath=newPath
		relativePath = filepath.Join(relativePath,"..")
		_,e=os.Stat(filepath.Join(relativePath,projectFile))
	}


	if b,err:=ioutil.ReadFile(filepath.Join(relativePath,projectFile));err!=nil {
		return nil, err;
	} else {
		if e:=json.Unmarshal(b,project);e!=nil {
			return project,e;
		}

	}
	return
}
func help() {
	fmt.Println("You need help, try init, status or sync. init -u <manifest git url> for the manifest")
}
// Assuming the base of the path is baseRepo
// the relative path is the number of "/" characters
// to the current folder, this will return a path which can be appended
// a path to
func relativeToRepo(path string) (string) {
	relativepath := "."
	for x:=0;x<strings.Count(path,"/")+1;x++ {
		relativepath = filepath.Join("..",relativepath)
	}
	return relativepath;
}
//func inList(item string, list []string) bool {
//	for _,i := range list {
//		if i==item {
//			return true
//		}
//	}
//	return false
//}
//
//func cp(src,dst string) error {
//	s, err := os.Open(src)
//	if err != nil {
//		return err
//	}
//	// no need to check errors on read only file, we already got everything
//	// we need from the filesystem, so nothing can go wrong now.
//	defer s.Close()
//	d, err := os.Create(dst)
//	if err != nil {
//		return err
//	}
//	if _, err := io.Copy(d, s); err != nil {
//		d.Close()
//		return err
//	}
//	return d.Close()
//}
//

//func repoInit() (err error) {
//	// Fetch the flags from the init
//	var url,branch string
//	mySet := flag.NewFlagSet("",flag.ExitOnError)
//	mySet.StringVar(&url,"u","","a url")
//	mySet.StringVar(&branch,"b","","a branch")
//	mySet.Parse(os.Args[2:])
//	if len(url)==0 {
//		fmt.Println("Url required")
//		return
//	}
//	//baseRepo :=".repo" //,_ :=filepath.Abs("./.repo")
//	os.RemoveAll(baseRepo)
//	os.MkdirAll(".repo/project-objects",os.ModePerm);
//
//	//var res *git.Repo
//	var args  []string
//	if len(branch)>0 {
//		args = append(args,"-b",branch)
//	}
//	if err = checkout(url,
//		filepath.Join(baseRepo,"manifests"),
//		filepath.Join(baseRepo,"manifests.git"),
//		args);err!=nil {
//		return;
//	}
//
//	// Link the manifest file to the one checked out
//	relativeToRepo(filepath.Join(baseRepo,"manifest.xml"))
//	os.Symlink(filepath.Join(relativeToRepo(filepath.Join(baseRepo,"manifest.xml")),baseRepo,"manifests","default.xml"),filepath.Join(baseRepo,"manifest.xml"))
//
//	// Read in the xml file, since the repo isnt a simple file read it into a map first
//	projectRepo := Repo{
//		Root: url,
//		ProjectMap:make(map[string]*Project),
//		RemoteMap:make(map[string]*Remote),
//	}
//	i :=strings.LastIndex(url,"/")
//	if i>0 {
//		/*
//		r:=strings.Split(url,"/")
//		projectRepo.Root=strings.Join(r[:len(r)-1],"")
//		*/
//		projectRepo.Root=url[:i]
//	}
//
//	if data,err:=ioutil.ReadFile(filepath.Join(baseRepo,"manifests","default.xml"));err!=nil {
//		return err
//	}   else {
//		if  err = projectRepo.Parse(filepath.Join(baseRepo,"manifests","default.xml"),data);err!=nil {
//			return err
//		}
//
//	}
//
//	// Dump the project repo to the baseRepo folder
//	if b,e:=json.Marshal(projectRepo);e!=nil {
//		return e;
//	} else {
//		err= ioutil.WriteFile(projectFile,b,os.ModePerm)
//	}
//   return
//}
//func repoSync() (err error){
//	args := os.Args[2:]
//	var projectRepo *Repo
//	if projectRepo,err=fetchRepoProject();err!=nil {
//		return
//	}
//
//	if len(args)==0 {
//		// Checkout remainder of projects
//		if err=projectRepo.CheckoutRepo();err!=nil {
//			fmt.Errorf("Unexpected errror checkout %#v\n",err)
//		} else {
//			if err=projectRepo.SyncRepo();err!=nil {
//				fmt.Errorf("Unexpected errror sync %#v\n",err)
//
//			}
//		}
//	}
//
//	// Now sync source with file system link the paths dynamically
//	// This should require nothing more then creating the path and making a symlink to the real git repo
////	if err=projectRepo.Checkout();err!=nil {
////		fmt.Errorf("Unexpected errror %#v\n",err)
////	}
//	return
//}
///*
//func (r *Repo) Checkout() (err error) {
//	// For each project checkout just the repository
//	// The repository will just checkout the git dir
//	projectObjects := filepath.Join(baseRepo,"project-objects")
//	for _,project:=range r.ProjectMap {
//		os.MkdirAll(project.Path,os.ModePerm)
//		a,_:=filepath.Abs(filepath.Join(projectObjects,project.Name))
//		if err = os.Symlink(a,filepath.Join(project.Path,".git"));err!=nil {
//			return err;
//		}
//
//	}
//	return
//}
//*/
//func (r *Repo) CheckoutRepo() (err error) {
//	// For each project checkout just the repository
//	// The repository will just checkout the git dir
//	fmt.Println("Checking out repositories")
//	for _,project:=range r.ProjectMap {
//		// Url is resolved, now we need to determine the path
//
//		if err=project.checkout();err!=nil {
//			return;
//		}
//		if _,e:=os.Stat(project.SecondaryGitPath);e!=nil {
//			if err = symlinkProject(false,project.PrimaryBareGitPath,project.SecondaryGitPath); err != nil {
//				return;
//			}
//		}
//		// Update the config file in the secondary repository with revision data
//		if err=project.updateSecondaryConfig();err!=nil {
//			return
//		}
//	}
//	return
//}
//
//func (r *Repo) SyncRepo() (err error) {
//	// For each project checkout just the repository
//	// The repository will just checkout the git dir
//
//	for _,project:=range r.ProjectMap {
//		// Url is resolved, now we need to determine the path
//
//		if err=project.syncLocal();err!=nil {
//			return
//		}
//	}
//	return
//}
//
//// Synchronize the local directory tree
//// Includes creating directories based on the projects path
//func (p *Project) syncLocal() (err error) {
//	// Check for existence of git folder, if it does not exist create a sim link for it, pointing to the secondary one
//	if _,e:=os.Stat(p.SourceGitPath);e!=nil {
//		os.MkdirAll(p.Path,os.ModePerm)
//		if err = symlinkProject(true, p.SecondaryGitPath, p.SourceGitPath);err!=nil {
//			return
//		}
//	}
//	GitCmdCall("checkout","",p.SourcePath,nil)
//
//	return
//}
//func (p *Project) checkout() (err error) {
//	if _,e:=os.Stat(p.PrimaryBareGitPath);e==nil {
//		err=fetch(p.Url, p.PrimaryBareGitPath,nil)
//	} else {
//		checkout(p.Url,p.PrimaryBareGitPath,"",[]string{"--bare"})
//	}
//
//
//	return;
//}
//type GitCmd struct {
//	Command string
//	Path string
//	WorkingDir string
//	Args []string
//}
//func (p *Project) updateSecondaryConfig() (err error) {
//	// Initialize git in the primary config
//	/*
//	if err=GitCmd(p.PrimaryBareGitPath,"init",nil);err!=nil {
//		return
//	}
//	*/
//	configPath := filepath.Join(p.SecondaryGitPath,"config")
//	commandList := []GitCmd{
//		{"init",p.PrimaryBareGitPath,"",nil},
//		{"config",p.PrimaryBareGitPath,"",[]string{"--file",configPath,"--unset-all","core.bare"}},
//		{"config",p.PrimaryBareGitPath,"",[]string{"--file",configPath,"--replace-all",fmt.Sprintf("remote.%s.url",p.RemoteKey),p.Url}},
//		{"config",p.PrimaryBareGitPath,"",[]string{"--file",configPath,"--replace-all",fmt.Sprintf("remote.%s.review",p.RemoteKey),p.Review}},
//		{"config",p.PrimaryBareGitPath,"",[]string{"--file",configPath,"--replace-all",fmt.Sprintf("remote.%s.projectname",p.RemoteKey),p.Name}},
//		{"config",p.PrimaryBareGitPath,"",[]string{"--file",configPath,"--replace-all",fmt.Sprintf("remote.%s.fetch",p.RemoteKey),fmt.Sprintf("+refs/heads/*:refs/remotes/%s/*",p.RemoteKey)}},
//		{"init",p.SecondaryGitPath,"",nil},
//		{"fetch",p.SecondaryGitPath,"",[]string{p.RemoteKey,"--tags",fmt.Sprintf("+%s:refs/remotes/%s/%s",p.Revision,p.RemoteKey,filepath.Base(p.Revision))}},
//		{"pack-refs",p.SecondaryGitPath,"",[]string{"--all","--prune"}},
//	}
//
//
//	return processGitCommandList(commandList)
//}
//
//// Called to update the project folder
//// creates a folder with the path specified in the p.SecondaryGitPath
//// Then copies all files to the shared folder
//// except the following files and folders will be linked using a relative path to the parent.
////    symlink_files = ['description', 'info']
////    symlink_dirs = ['hooks', 'objects', 'rr-cache', 'svn']
////    if share_refs:
////      # These objects can only be used by a single working tree.
////      symlink_files += ['config', 'packed-refs', 'shallow']
////      symlink_dirs += ['logs', 'refs']
//func symlinkProject(ref bool,source,destination string) (err error) {
//	os.MkdirAll(destination,os.ModePerm)
//	// Store relative path to this repo
//	rpath := relativeToRepo(destination);
//	//copySource := path.Dir(source)
//	//copyDestination := path.Dir(destination)
//
//	// Create missing content in master to provided links for
//	files := reposymlinkFile
//	dirs := reposymlinkDir
//	if ref {
//		files = reposymlinkRefFile
//		dirs = reposymlinkRefDir
//	}
//	for _, file := range files {
//		newFile := filepath.Join(source, file)
//		if _,e:=os.Stat(newFile);e!=nil {
//			// Missing file create an empty one
//			if err=ioutil.WriteFile(newFile,nil,os.ModePerm);err!=nil {
//				return
//			}
//		}
//	}
//	for _, dir := range dirs {
//		newDir := filepath.Join(source, dir)
//		if _,e:=os.Stat(newDir);e!=nil {
//			// Missing file create an empty one
//			if err=os.MkdirAll(newDir,os.ModePerm);err!=nil {
//				return
//			}
//		}
//	}
//
//	if content,err:=ioutil.ReadDir(source);err!=nil {
//		return err
//	} else {
//		for _, fullFile := range content {
//			file := fullFile.Name()
//			var symLink bool
//			if ref {
//				symLink = inList(file, reposymlinkRefFile) || inList(file, reposymlinkRefDir)
//			} else {
//				symLink = inList(file, reposymlinkFile) || inList(file, reposymlinkDir)
//			}
//
//			if symLink {
//				if err = os.Symlink(filepath.Join(rpath, source, file), filepath.Join(destination, file)); err != nil {
//					return err;
//				}
//			}  else {
//				// Copy the file
//				if !fullFile.IsDir() {
//					if err = cp(filepath.Join(source, file), filepath.Join(destination, file)); err != nil {
//						return err
//					}
//				}
//			}
//
//		}
//	}
//
//	return
//	/*
//	os.MkdirAll(p.SecondaryGitPath,os.ModePerm)
//	// Store relative path to this repo
//	rpath := relativeToRepo(p.SecondaryGitPath);
//
//	if err = os.Symlink(filepath.Join(rpath,p.PrimaryBareGitPath,"hooks"),filepath.Join(p.SecondaryGitPath,"hooks"));err!=nil {
//		return err;
//	}
//	fmt.Println("Relative",rpath, p.SecondaryGitPath)
//
//	return
//	*/
//}
