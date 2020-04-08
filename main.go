package main

import (
	"crypto/md5"
	"flag"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws/credentials"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
)

func main() {
	// configure and connect to a local dynamodb instance.
	// to create a test instance with a shared database, try the following docker command:
	//    docker run --name dynamo-local --rm -p 8000:8000 amazon/dynamodb-local -jar DynamoDBLocal.jar -sharedDb
	//
	// note that when using '-sharedDb', the credential values don't matter (but must be informed anyway)
	// you should also import the schema and data defined in TodoApp.json. An easy way to do this is by using
	// NoSql Workbench to connect to your previously-created local instance, load the 'TodoApp.json' file and commit
	// the changes to your local instance.
	config := aws.Config{
		Region:      aws.String("sa-east-1"),
		Credentials: credentials.NewStaticCredentials("id", "secret", ""),
		Endpoint:    aws.String("http://localhost:8000"),
	}

	s := session.Must(session.NewSession(&config))
	db := dynamodb.New(s)

	// define and load command-line parameters
	text := flag.String("text", "", "Sets the text of the TODO")
	key := flag.String("key", "", "Sets the key of the TODO. If not specified, is auto generated.")
	add := flag.String("add", "", "Adds a new TODO item, with the specified text (shortcut for -key and -text)")
	context := flag.String("context", "INBOX", "Default context to use")
	tags := flag.String("tags", "", "Attach the comma-separated list of tags to the specified TODO.")
	done := flag.Bool("done", false, "Marks/unmarks the TODO as done")

	flag.Parse()

	// when -add is used, key is autogenerated and text is overriden
	if *add != "" {
		*key = ""
		*text = *add
	}

	// context is always uppercase
	if *context == "" && *key == "" {
		*context = "INBOX"
	}
	*context = strings.ToUpper(*context)

	switch {
	case *key == "" && *text != "":
		fallthrough
	case *add != "":
		addTodo(db, *key, *context, *text, *tags, *done)

	case *key != "" && *text != "":
		updateTodo(db, *key, *context, *text, *tags, *done)

	case *tags != "":
		listByTags(db, *tags, *context, *done)

	default:
		listByContext(db, *key, *context, *done)
	}
}

// listByContext lists all the todos of a given context. By default, the 'INBOX' context
// is used, and only the pending tasks are shown. It is also possible to filter for a
// specific key
func listByContext(db *dynamodb.DynamoDB, key, context string, isDone bool) {
	sk := boolToStatus(isDone)

	if key != "" {
		sk += "#" + key
	}

	input := dynamodb.QueryInput{
		TableName:              aws.String("todos"),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("context = :context and begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":context": {
				S: aws.String(context),
			},
			":sk": {
				S: aws.String(sk),
			},
		},
	}

	res, err := db.Query(&input)
	if err != nil {
		panic(err)
	}

	if *res.Count == int64(0) {
		fmt.Println("No results found.")
		return
	}

	fmt.Printf("Showing tasks on context '%s', with isDone '%v'.\n", context, isDone)
	if key != "" {
		fmt.Printf("Further filtering using key '%s'.\n", key)
	}
	fmt.Println()

	printResults(res.Items)
}

// listByTags mounts a listing based on a given tag. It is possible to join
// more than one tag in the result list, as well as exclude specific tags
// by prepending a "-" on the tag name.
//
// The default context is 'INBOX', and only the pending TODOS are shown.
func listByTags(db *dynamodb.DynamoDB, tags, context string, isDone bool) {
	ts := strings.Split(tags, ",")
	keys := make(map[string]struct{})

	for _, tag := range ts {
		if !fillKeysWithTag(keys, db, tag, isDone) {
			fmt.Println("WARNING: Maximum item limit reached. Results will be truncated.")
			break
		}
	}

	if len(keys) == 0 {
		fmt.Println("No results found.")
		return
	}

	// build & run the query
	input := dynamodb.QueryInput{
		TableName:              aws.String("todos"),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("context = :context"),
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":context": {
				S: aws.String(context),
			},
		},
	}

	res, err := db.Query(&input)
	if err != nil {
		panic(err)
	}

	// since our filter is SK (that is part of primaty key)
	// we cannot use filter on the query, we need to solve this on the
	// client side
	items := make([]map[string]*dynamodb.AttributeValue, 0, len(res.Items))

	for _, item := range res.Items {
		sk := *item["SK"].S

		if _, ok := keys[sk]; ok {
			items = append(items, item)
		}
	}

	if len(items) == 0 {
		fmt.Println("No results found.")
		return
	}

	fmt.Printf("Showing tasks with tags '%s' on context '%s', with isDone '%v'.\n", tags, context, isDone)
	printResults(items)
}

func fillKeysWithTag(keys map[string]struct{}, db *dynamodb.DynamoDB, tag string, isDone bool) bool {
	isRemove := strings.HasPrefix(tag, "-")

	if isRemove {
		tag = tag[1:]
	}

	// no need to remove from an empty list
	if isRemove && len(keys) == 0 {
		return true
	}

	input := dynamodb.QueryInput{
		TableName:              aws.String("todos"),
		KeyConditionExpression: aws.String("PK = :pk and begins_with(SK, :sk)"),
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":pk": {
				S: aws.String("TAG#" + tag),
			},
			":sk": {
				S: aws.String(boolToStatus(isDone)),
			},
		},
	}

	res, err := db.Query(&input)
	if err != nil {
		panic(err)
	}

	for _, item := range res.Items {
		key := *item["SK"].S
		keys[key] = struct{}{}
	}

	return true
}

func addTodo(db *dynamodb.DynamoDB, key, context, text, tags string, isDone bool) {
	if key == "" {
		key = generateID(text)
	}
	sk := boolToStatus(isDone)

	input := dynamodb.PutItemInput{
		TableName: aws.String("todos"),
		Item: map[string]*dynamodb.AttributeValue{
			"PK": {
				S: aws.String("TODO#" + key),
			},
			"SK": {
				S: aws.String(sk + "#" + key),
			},
			"name": {
				S: aws.String(text),
			},
			"context": {
				S: aws.String(context),
			},
			"tags": {
				S: aws.String(tags),
			},
		},
	}

	if _, err := db.PutItem(&input); err != nil {
		panic(err)
	}

	ts := strings.Split(tags, ",")

	for _, tag := range ts {
		input.Item = map[string]*dynamodb.AttributeValue{
			"PK": {
				S: aws.String("TAG#" + tag),
			},
			"SK": {
				S: aws.String(sk + "#" + key),
			},
		}

		if _, err := db.PutItem(&input); err != nil {
			panic(err)
		}
	}
}

func updateTodo(db *dynamodb.DynamoDB, key, context, text, tags string, isDone bool) {
	// find the current value. This is needed to remove the tags
	input := dynamodb.GetItemInput{
		TableName: aws.String("todos"),
		Key: map[string]*dynamodb.AttributeValue{
			"PK": {
				S: aws.String("TODO#" + key),
			},
			"SK": {
				S: aws.String("PENDING#" + key),
			},
		},
	}

	item, err := db.GetItem(&input)
	if err != nil {
		panic(err)
	}

	if len(item.Item) == 0 {
		fmt.Println("No item found.")
	}

	// the lazy solution: just rewrite everything.
	// on real-world, this should be optimized
	sk := *item.Item["SK"].S
	oldtags := strings.Split(*item.Item["tags"].S, ",")

	for _, t := range oldtags {
		delInput := dynamodb.DeleteItemInput{
			TableName: aws.String("todos"),
			Key: map[string]*dynamodb.AttributeValue{
				"PK": {
					S: aws.String("TAG#" + t),
				},
				"SK": {
					S: aws.String(sk),
				},
			},
		}

		if _, err := db.DeleteItem(&delInput); err != nil {
			panic(err)
		}
	}

	delInput := dynamodb.DeleteItemInput{
		TableName: aws.String("todos"),
		Key: map[string]*dynamodb.AttributeValue{
			"PK": {
				S: aws.String("TODO#" + key),
			},
			"SK": {
				S: aws.String(sk),
			},
		},
	}

	if _, err := db.DeleteItem(&delInput); err != nil {
		panic(err)
	}

	// now, just re-add the item
	if tags == "" {
		tags = *item.Item["tags"].S
	}

	if context == "" {
		context = *item.Item["context"].S
	}

	addTodo(db, key, context, text, tags, isDone)
}

// aux. funcs

func printResults(res []map[string]*dynamodb.AttributeValue) {
	fmt.Printf("%4s  %-30s  %s\n", "ID", "TASK", "TAGS")
	fmt.Printf("%4s  %-30s  %s\n", "----", "----", "----")

	for _, item := range res {
		id := strings.TrimPrefix(*item["PK"].S, "TODO#")
		name := *item["name"].S
		tags := *item["tags"].S
		fmt.Printf("%4s  %-30s  %s\n", id, name, tags)
	}

	fmt.Printf("\nTotal records: %d\n", len(res))
}

func boolToStatus(value bool) string {
	if value {
		return "DONE"
	}

	return "PENDING"
}

func generateID(text string) string {
	// yeah, I know that this has an extremely high collision chance.
	// but, for sample purposes, it is good enough
	h := md5.New()
	_, _ = h.Write([]byte(text))
	b := h.Sum(nil)
	s := fmt.Sprintf("%x", b)
	return s[:4]
}