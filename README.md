# repo-tool
A tool to handle multiple git repositories.

* Requirements
git and go

This tool uses the same format for configuration manifest files that google uses with its [python tool](https://code.google.com/p/git-repo/)
Note it is *NOT* compatible with a project checked out using the git-repo tool, it only reads the same type of
[manifest files](https://android.googlesource.com/tools/repo/+/v1.12.20/docs/manifest-format.txt)

As time passes this will become more complete, currently it only works with a single manifest (default.xml), you can do something like

    repo-tool init -u git@github.com/notzippy/some_test_manifest.git
    
and volia you have a project folder with your repository tree nicely checked out in it.
Besides `repo-tool init` and `repo-tool sync` `repo-tool status` is also supported to let you know what state all the repositories are in.

Your manifest file can have different branches, based on your manifest branch you can checkout different branches for your project. For example look at this [manifest](https://github.com/notzippy/revel-manifest/blob/master/default.xml) and this [one](https://github.com/notzippy/revel-manifest/blob/develop/default.xml). If you have initialized the repo with the first you can switch to the develop branch (of the manifest, which will automatically switch all the projects to the appropriate branch as well). Like this

    repo-tool sync -b develop

Future work to do (in no particular order)
1. Support committing all or list of repositories
2. Support multiple manifest files (and see if remove-project works)
3. Check windows support (it should work)
4. Support running an arbitrary git command against all the projects


[[Details]]

Each project is checked out using the url, revision and path specified in the project manifest file. When the project is checked out
the branch is not in a detached state, it is pointing to the branch specified in the manifest file.

[Updates]
* Add hg support, repotype attribute supported in both project and remote levels. Defaults to "git" supports "hg" as well
* Added -j flag, applicable to both sync and init, defaults to 1 job (ie checkout one project at a time), can be set higher sub in a number after the j for example -j4 (checkout 4 projects at a time)
* Added -g flag, applicable to init, checkout specific groups see manifest for details
* Added switching based on branch of manifest file

