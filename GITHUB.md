# Reminder to self to upload to github.com

Create Repo On GitHub

* On GitHub website:
* New Repository
* Name: discover
* Do NOT initialize with README
* Create

Push code to newly created repository:

```
cd ~/dev/discover/
ls
git status
git init
git add .
git commit -m "Initial public commit of discover project, starting with version 1.4"
git remote add origin git@github.com:luxzg/discover.git
git remote set-url origin git@github.com:luxzg/discover.git
git branch -M main
git push -u origin main
```

Check online: https://github.com/luxzg/discover
