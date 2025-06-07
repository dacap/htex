## docs

### htex elements

* [<!content>](#content)
* [<!data>](#data-formfield)
* [<!include-escaped>](#include-escaped-file)
* [<!include-markdown>](#include-markdown-file)
* [<!include-raw>](#include-raw-file)
* [<!layout>](#layout-file)
* [<!method>](#method-httpmethod)
* [<!query>](#query-key)

#### <!content>

Can be used inside a layout to insert the page content in some place
inside the layout template. If the layout is accessed directly, this
is replaced with just an empty string.

#### <!data formfield>

It's replaced with the value of the given `formfield`.

#### <!include-escaped file>

Includes the content of the given `file` in the output escaping the
HTML characters, e.g. useful to show the source code of a file.

#### <!include-markdown file>

Converts the given markdown file to HTML. This uses
[github.com/gomarkdown/markdown](https://github.com/gomarkdown/markdown)
library.

#### <!include-raw file>

Includes the content of the given `file` in the output just as it is,
i.e. without processing it.

#### <!layout file>

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

#### <!method httpmethod>

```
<!method get>
<!method get key
<!method get key1=value1 key2=value2 ...>
<!method post>
<!method ...>
```

Filters content depending on the current method in the HTTP request,
all the following elements will be ignored until a new `<!method>` is
found. If a `key` alone is specified, only requests with that given
`key` in the query URL (e.g. `?key`) will be output the section. The
same goes for `key=value` including the section when `key` is equal to
`value`.

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

#### <!query key>

It's replaced with the value of the given `key` from the URL
query. E.g. If we access `/path/?id=2` in the following example
```html
user ID is <!query id>
```
we get
```html
user ID is 2
```
