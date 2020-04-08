# dynamo-todo

This is a little demo application o mess around a bit with Amazon DynamoDB.

Nothing fancy to see here, this is just a semi-usable command-line TODO list that reads/writes into DynamoDB.
It is useful only to quickly remember how to do basic operations in the API.

## Building

First compile the project (a simple `go build` is enough) and make sure you have a local DynamoDB instance running (so you don't waste money running this on the real thing...):

```bash
# compile the project
go build

# run a local, disposable dynamo instance
docker run --name dynamo-local --rm -p 8000:8000 amazon/dynamodb-local -jar DynamoDBLocal.jar -sharedDb
```

Remember that you will have an empty database at this stage. The last thing you need to do is to use [NoSQL Workbench](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/workbench.settingup.html) to load the model definition of the file `TodoApp.json` and commit that to your running instance.

Linux users don't have the luxury of using NoSQL Workbench and must create the schema and/or load the data manually. Luckly, the `DataModel` and `TableData` keys of the json file have all the information (and practically on the same format) that you need to do so. Have fun!

## Usage samples

```bash
$ ./dynamo-todo

Usage of ./dynamo-todo:
  -add string
        Adds a new TODO item, with the specified text (shortcut for -key and -text)
  -context string
        Default context to use (default "INBOX")
  -done
        Marks/unmarks the TODO as done
  -key string
        Sets the key of the TODO. If not specified, is auto generated.
  -tags string
        Attach the comma-separated list of tags to the specified TODO.
  -text string
        Sets the text of the TODO

# listing, with default options
./dynamo-todo

# list from a specific context
./dynamo-todo -context NEXT

# list by tags, in a specific context
./dynamo-todo -tags home,work -context NEXT

# add an item with a specific tag
./dynamo-todo -text "Review emails" -tags work

# updating an item
./dynamo-todo -key 2 -text "Hey ho!" -tags fun,vacation
```

## License

MIT. In short, have fun.
