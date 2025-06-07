# htex - hypertext extruder

[![MIT Licensed](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE.txt)

## introduction

An experimental tool which is:

* an HTML-extension to generate hypertext (new set of `<!elements>`),
* a web server to publish static and dynamic content (`htex server`),
* a static site generator like jekyll or hugo (`htex gen`).

## quick start

Clone htex repository

    git clone github.com/dacap/htex

and execute

    cd htex
    go run ./cmd/htex server

to run a local server (http://localhost:80) for the given
[`public` folder](public/) content.

You can modify the [`./public/index.htex` file](public/index.htex):
```html
<html>
<body>
  <!method get>
    <form action="." method="post">
      <input type="email" name="email" placeholder="email">
      <input type="password" name="password" placeholder="password">
      <button type="submit">sign in</button>
    </form>
  <!method post>
    Form received: <!data email>
  <!method any>
</body>
</html>
```

Files ending with `.htex` will create a route to access that URL path,
e.g. `public/hi.htex` will receive requests to `/hi/`, and the same if
you create `public/hi/index.htex`. Any other file will be served as
static content, and the user will not be able to download the source
of `.htex` files directly (not even using `/hi/index.htex` in the HTTP
request).

Hidden files and directories are not be published (returning 404
code), unless the file is inside the `.well-known` directory, which is
used for domains/certificate validations.

## docs

Go to [public/docs/docs/](public/docs/docs.md).
