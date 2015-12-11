// (c) 2015 Stani Michiels

/*
Package typewriter can rewrite types in go source code by modifying
the AST syntax tree. As such the same code/api can be reused for
different types.

The package 'typewriter' does not contain any code itself, but only
this documentation.
The package 'packages' is a generic library to convert any type to
any type. If you know what you are doing, you could use and import
this package, however more likely you will just want to use the
gofloat command.
The gofloat command can translate any source code from integers to
floats if a json configuration file is given. You need to install it
first, before you can use it:

  cd gofloat
  go install

See the documentation of 'gofloat' and 'packages' for more information
and limitations.

Case study - svgo

As a case study svgo can be translated from integer coordinates/values
to floats, with the provided gofloat command:

  gofloat svgo.json

As a test the
svgo_test.sh bash script can be run to generate the different examples
with the generated float64 version of svgo (svgofloat).

*/
package typewriter
