package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/google/go-github/v29/github"
	"github.com/jedib0t/go-pretty/table"
	"golang.org/x/oauth2"
)

// Issue contains the information for a GitHub issue that we care about for planning purposes.
type Issue struct {
	Issue     string
	Milestone string
	Assignee  string
	Points    int
	URL       string
}

var (
	flagGitHubOrganization = flag.String("org", "default", "GitHub organization to scan.")
	flagGitHubLabel        = flag.String("label", "default", "GitHub label marking all issues to be included.")
	flagAll                = flag.Bool("all", false, "Prints all issues in one table")
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

	// https://developer.github.com/v3/issues/#list-issues
	opts := &github.IssueListOptions{
		ListOptions: github.ListOptions{
			PerPage: 50,
		},
		Filter: "all",
		State:  "open",
		Labels: []string{*flagGitHubLabel},
	}
	var allIssues []Issue

	for {
		issues, resp, err := client.Issues.ListByOrg(ctx, *flagGitHubOrganization, opts)
		if err != nil {
			log.Fatalf("error listing GitHub issues: %v", err)
		}

		for _, i := range issues {
			allIssues = append(allIssues, createIssue(i))
		}

		// Fetch the next page of results as needed.
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	// We only print the all issues view, if the all flag was specified.
	if flagAll != nil && *flagAll {
		renderTable(allIssues, "All Issues")
		return
	}

	fmt.Println("\n# Points Per-Assignee\n")
	renderPerAssignee(allIssues)

	fmt.Println("\n# Points Per-Milestone\n")
	renderPerMilestone(allIssues)
}

func createIssue(gitHubIssue *github.Issue) Issue {
	points := getSizeValue(gitHubIssue)

	var milestoneStr string
	if milestone := gitHubIssue.GetMilestone(); milestone != nil {
		milestoneStr = milestone.GetTitle()
	} else {
		milestoneStr = "(none)"
	}

	var assigneeStr string
	if assignee := gitHubIssue.GetAssignee(); assignee != nil {
		assigneeStr = assignee.GetLogin()
	}
	return Issue{
		Issue:     gitHubIssue.GetTitle(),
		Milestone: milestoneStr,
		Assignee:  assigneeStr,
		Points:    points,
		URL:       gitHubIssue.GetHTMLURL(),
	}
}

func getSizeValue(issue *github.Issue) int {
	for _, l := range issue.Labels {
		switch size := l.GetName(); size {
		case "size-s", "size/S":
			return 1
		case "size-m", "size/M":
			return 5
		case "size-l", "size/L":
			return 10
		default:
			continue
		}
	}
	// The issue does not have a size label. We will return a high value to
	// make sure we realize something is up.
	return 500
}

func renderTable(issues []Issue, headerText string) {
	pointsPerMilestone := make(map[string]int)

	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.AppendHeader(table.Row{headerText})
	t.AppendHeader(table.Row{"Issue", "Milestone", "Assignee", "Points", "URL"})
	var sumPoints int

	sortIssues(issues)
	for _, i := range issues {
		sumPoints += i.Points
		t.AppendRow([]interface{}{i.Issue, i.Milestone, i.Assignee, i.Points, i.URL})
		pointsPerMilestone[i.Milestone] += i.Points
	}

	t.AppendFooter(table.Row{"", "", "Total", sumPoints})

	// Sort per-milestone totals.
	sortedMilestones := make([]string, 0, len(pointsPerMilestone))
	for k := range pointsPerMilestone {
		sortedMilestones = append(sortedMilestones, k)
	}
	sort.Strings(sortedMilestones)

	for _, milestone := range sortedMilestones {
		points := pointsPerMilestone[milestone]
		milestoneText := fmt.Sprintf("Points for %s", milestone)
		t.AppendFooter(table.Row{"", "", milestoneText, points})
	}
	t.Render()
}

func groupByAssignee(issues []Issue) map[string][]Issue {
	issuesByAssignee := make(map[string][]Issue)

	for _, i := range issues {
		if val, ok := issuesByAssignee[i.Assignee]; ok {
			issuesByAssignee[i.Assignee] = append(val, i)
		} else {
			issuesByAssignee[i.Assignee] = []Issue{i}
		}
	}

	return issuesByAssignee
}

func groupByMilestone(issues []Issue) map[string][]Issue {
	issuesByMilestone := make(map[string][]Issue)

	for _, i := range issues {
		if val, ok := issuesByMilestone[i.Milestone]; ok {
			issuesByMilestone[i.Milestone] = append(val, i)
		} else {
			issuesByMilestone[i.Milestone] = []Issue{i}
		}
	}

	return issuesByMilestone
}

func renderPerAssignee(issues []Issue) {
	issuesByAssignee := groupByAssignee(issues)

	// Get assignees, then sort.
	sortedAssignees := make([]string, 0, len(issuesByAssignee))
	for k := range issuesByAssignee {
		sortedAssignees = append(sortedAssignees, k)
	}
	sort.Strings(sortedAssignees)

	for _, assignee := range sortedAssignees {
		assignedIssues := issuesByAssignee[assignee]
		renderTable(assignedIssues, fmt.Sprintf("Issues for %s", assignee))

		// Separate each individual person.
		fmt.Println("\n")
	}
}

// renderPerMilestone breaks things down with milestones as columns, and assignees as rows.
func renderPerMilestone(allIssues []Issue) {
	// Get assignees, then sort.
	issuesByAssignee := groupByAssignee(allIssues)
	sortedAssignees := make([]string, 0, len(issuesByAssignee))
	for k := range issuesByAssignee {
		sortedAssignees = append(sortedAssignees, k)
	}
	sort.Strings(sortedAssignees)

	// Get milestones, then sort.
	issuesByMilestone := groupByMilestone(allIssues)
	sortedMilestones := make([]string, 0, len(issuesByMilestone))
	for k := range issuesByMilestone {
		sortedMilestones = append(sortedMilestones, k)
	}
	sort.Strings(sortedMilestones)

	// Now we render the table. We iterate through the assignees, but
	// build up the columns dynamically, grouping from all issues
	// assigned to that user.
	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.AppendHeader(table.Row{"Total for Team"})
	headerRows := table.Row{"Assignee"}
	for _, milestone := range sortedMilestones {
		headerRows = append(headerRows, milestone)
	}
	t.AppendHeader(headerRows)

	totalPointsByMilestone := make(map[string]int)
	// For each assignee, break down issue totals by milestone.
	for _, assignee := range sortedAssignees {
		rowValues := []interface{}{assignee}

		milestoneTotals := make(map[string]int)
		assigneeIssuesByMilestone := groupByMilestone(issuesByAssignee[assignee])
		for milestone, issuesInMilestone := range assigneeIssuesByMilestone {
			var totalForMilestone int
			for _, issueInMilestone := range issuesInMilestone {
				totalForMilestone += issueInMilestone.Points
			}
			milestoneTotals[milestone] = totalForMilestone

			// Add this individual's per-milestone total to the aggregate total.
			if _, ok := totalPointsByMilestone[milestone]; !ok {
				totalPointsByMilestone[milestone] = totalForMilestone
			} else {
				totalPointsByMilestone[milestone] = totalPointsByMilestone[milestone] + totalForMilestone
			}
		}

		for _, milestone := range sortedMilestones {
			rowValues = append(rowValues, milestoneTotals[milestone])
		}
		t.AppendRow(rowValues)
	}

	// Render the footer, with totals for the whole team.
	footerRowValues := table.Row{"Total For Team"}
	for _, milestone := range sortedMilestones {
		footerRowValues = append(footerRowValues, totalPointsByMilestone[milestone])
	}
	t.AppendFooter(footerRowValues)

	t.Render()
}

// sortIssues will sort the provided slice by assignee, milestone, size, then name.
func sortIssues(issues []Issue) {
	sort.SliceStable(issues, func(i, j int) bool {
		issueI, issueJ := issues[i], issues[j]
		if strings.Compare(issueI.Milestone, issueJ.Milestone) < 0 {
			return true
		}

		if strings.Compare(issueI.Assignee, issueJ.Assignee) < 0 {
			return false
		}
		if issueI.Points != issueJ.Points {
			return issueI.Points > issueJ.Points // Higher point values first.
		}
		return strings.Compare(issueI.Issue, issueJ.Issue) < 0
	})
}
