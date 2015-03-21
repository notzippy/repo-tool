# Manifest

Most functionality is imported from the design document for git-repo from here
[manifest files](https://android.googlesource.com/tools/repo/+/v1.12.20/docs/manifest-format.txt)

Understood manifest (Hopefully)
-------------------------------

	 <!DOCTYPE manifest [
		<!ELEMENT manifest (notice?,
				remote*,
				default?,
				remove-project*,
				project*)>

		  <!ELEMENT notice (#PCDATA)>
			<!ELEMENT remote (EMPTY)>
			<!ATTLIST remote name ID #REQUIRED>
			<!ATTLIST remote fetch CDATA #REQUIRED>
			<!ATTLIST remote revision CDATA #IMPLIED>

		 <!ELEMENT default (EMPTY)>
			<!ATTLIST default remote IDREF #IMPLIED>
			<!ATTLIST default revision CDATA #IMPLIED>

		 <!ELEMENT project (annotation*, project*)>
			<!ATTLIST project name CDATA #REQUIRED>
			<!ATTLIST project path CDATA #IMPLIED>
			<!ATTLIST project remote IDREF #IMPLIED>
			<!ATTLIST project revision CDATA #IMPLIED>

		 <!ELEMENT remove-project (EMPTY)>
			<!ATTLIST remove-project name CDATA #REQUIRED>

Element manifest
----------------

The root element of the file.

Element remote
--------------
One or more remote elements may be specified. Each remote element
specifies a Git URL shared by one or more projects.

Attribute `name`: A short name unique to this manifest file. The
name specified here is used as the remote name in each project's
.git/config, and is therefore automatically available to commands
like `git fetch`, `git remote`, `git pull` and `git push`.

Attribute `fetch`: The Git URL prefix for all projects which use
this remote. Each project's name is appended to this prefix to
form the actual URL used to clone the project.

Attribute `revision`: Name of a Git branch (e.g. `master` or
`refs/heads/master`). Remotes with their own revision will override
the default revision.

Element default
---------------
At most one default element may be specified. Its remote and
revision attributes are used when a project element does not
specify its own remote or revision attribute.

Attribute `remote`: Name of a previously defined remote element.
Project elements lacking a remote attribute of their own will use
this remote.

Attribute `revision`: Name of a Git branch (e.g. `master` or
`refs/heads/master`). Project elements lacking their own
revision attribute will use this revision.

Element project
---------------
One or more project elements may be specified. Each element
describes a single Git repository to be cloned into the repo
client workspace.

Attribute `name`: A unique name for this project. The project's
name is appended onto its remote's fetch URL to generate the actual
URL to configure the Git remote with. The URL gets formed as:
${remote_fetch}/${project_name}.git
where ${remote_fetch} is the remote's fetch attribute and
${project_name} is the project's name attribute. The suffix ".git"
is always appended as repo assumes the upstream is a forest of
bare Git repositories. If the project has a parent element, its
name will be prefixed by the parent's.

Attribute `path`: An optional path relative to the top directory
of the repo client where the Git working directory for this project
should be placed. If not supplied the project name is used.
If the project has a parent element, its path will be prefixed
by the parent's.

Attribute `remote`: Name of a previously defined remote element.
If not supplied the remote given by the default element is used.

Attribute `revision`: Name of the Git branch the manifest wants
to track for this project. Names can be relative to refs/heads
(e.g. just "master") or absolute (e.g. "refs/heads/master").
Tags and/or explicit SHA-1s should work in theory, but have not
been extensively tested. If not supplied the revision given by
the remote element is used if applicable, else the default
element is used.

Element remove-project
----------------------
Deletes the named project from the internal manifest table, possibly
allowing a subsequent project element in the same manifest file to
replace the project with a different source.

This element is mostly useful in a local manifest file, where
the user can remove a project, and possibly replace it with their
own definition.

