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
    go run ./cmd/htex

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

## htex elements

* [<!content>](#content)
* [<!data>](#data-formfield)
* [<!include-raw>](#include-raw-file)
* [<!include-escaped>](#include-escaped-file)
* [<!layout>](#layout-file)
* [<!method>](#method-httpmethod)

## <!content>

Can be used inside a layout to insert the page content in some place
inside the layout template. If the layout is accessed directly, this
is replaced with just an empty string.

## <!data formfield>

It's replaced with the value of the given `formfield`.

## <!include-raw file>

Includes the content of the given `file` in the output just as it is,
i.e. without processing it.

## <!include-escaped file>

Includes the content of the given `file` in the output escaping the
HTML characters, e.g. useful to show the source code of a file.

## <!layout file>

Specify the layout related to the current `.htex` file. This is
generally specified at the top of the file.

Example for a `layout.htex` file:
```html
<html>
  <body>
    <!content>
  </body>
</html>
```
and `index.htex` file:
```html
<!layout layout.htex>
<p>Hello World</p>
```
the output will be:
```html
<html>
  <body>
    <p>Hello World</p>
  </body>
</html>
```

## <!method httpmethod>

Filters content depending on the current method in the HTTP request,
all the following elements will be ignored until a new `<!method>` is
found.

You can use `<!method any>` to go back to content that will be
displayed in any case.

Example:
```html
<body>
we are processing the
<!method get>    GET method
<!method post>   POST method
<!method put>    PUT method
<!method delete> DELETE method
<!method any>
</body>
```
