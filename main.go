package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/google/go-github/v29/github"
	"golang.org/x/oauth2"
)

var (
	flagGitHubOrganization = flag.String("org", "default", "GitHub organization to scan.")
	flagGitHubLabel        = flag.String("label", "default", "GitHub label marking all issues to be included.")
)

func main() {
	flag.Parse()

	token := os.Getenv("GITHUB_AUTH_TOKEN")
	if token == "" {
		log.Fatal("Unauthorized: No token present")
	}

	if flagGitHubOrganization == nil || *flagGitHubOrganization == "" {
		log.Fatal("Error: Required --org flag not proided.")
	}
	if flagGitHubLabel == nil || *flagGitHubLabel == "" {
		log.Fatalf("Error: Required --label flag not provided.")
	}

	log.Printf(
		"Scanning GitHub organization %q and all issues labeled %q...",
		*flagGitHubOrganization, *flagGitHubLabel)

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	var sumPoints int

	// https://developer.github.com/v3/issues/#list-issues
	opts := &github.IssueListOptions{
		ListOptions: github.ListOptions{
			PerPage: 50,
		},
		Filter: "all",
		State:  "open",
		Labels: []string{*flagGitHubLabel},
	}

	for {
		issues, resp, err := client.Issues.ListByOrg(ctx, *flagGitHubOrganization, opts)
		if err != nil {
			log.Fatalf("error listing GitHub issues: %v", err)
		}

		// Columns
		fmt.Printf("%-70s\t%20s\t%20s\t%8s\t%s\n",
			"Issue", "Milestone", "Assignee", "Points", "URL")
		fmt.Printf("%-70s\t%20s\t%20s\t%8s\t%s\n",
			"---", "---", "---", "---", "---")

		for _, i := range issues {
			points := getSizeValue(i)
			sumPoints += points

			var milestoneStr string
			if milestone := i.GetMilestone(); milestone != nil {
				milestoneStr = milestone.GetTitle()
			}

			var assigneeStr string
			if assignee := i.GetAssignee(); assignee != nil {
				assigneeStr = assignee.GetName()
			}

			fmt.Printf("%-70s\t%20s\t%20s\t%8d\t%s\n",
				i.GetTitle(), milestoneStr, assigneeStr, points, i.GetHTMLURL())
		}

		// Fecth the next page of results as needed.
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	fmt.Printf("\n\nTOTAL SUM: %d\n", sumPoints)
	fmt.Printf("AVG PER MILESTONE: %d\n", sumPoints/3.0)
}

// getSizeValue inspects the GitHub issue and returns the number of "points"
// it is estimated to be.
func getSizeValue(issue *github.Issue) int {
	for _, l := range issue.Labels {
		switch size := l.GetName(); size {
		case "size-s":
			return 1
		case "size-m":
			return 5
		case "size-l":
			return 10
		default:
			continue
		}
	}
	// The issue does not have a size label. We will return a high value to
	// make sure we realize something is up.
	return 500
}
