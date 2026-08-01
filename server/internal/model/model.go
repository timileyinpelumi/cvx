package model

import (
	"fmt"
	"regexp"
	"strings"
)

type Bullet struct {
	ID     string   `json:"id"`
	Text   string   `json:"text"`
	Skills []string `json:"skills"`
}

type Item struct {
	ID           string   `json:"id"`
	Kind         string   `json:"kind"`
	Title        string   `json:"title"`
	Organization string   `json:"organization"`
	StartDate    string   `json:"startDate"`
	EndDate      string   `json:"endDate"`
	Bullets      []Bullet `json:"bullets"`
}

type Link struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type Profile struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Location string `json:"location"`
	Summary  string `json:"summary"`
	Links    []Link `json:"links"`
	Skills   []string `json:"skills"`
	Items    []Item `json:"items"`
}

type TBullet struct {
	SourceBulletID string `json:"sourceBulletId"`
	Text           string `json:"text"`
}

type TItem struct {
	SourceID     string    `json:"sourceId"`
	Title        string    `json:"title"`
	Organization string    `json:"organization"`
	Dates        string    `json:"dates"`
	Bullets      []TBullet `json:"bullets"`
}

type TSection struct {
	Title string  `json:"title"`
	Items []TItem `json:"items"`
}

type Gap struct {
	Requirement string `json:"requirement"`
	Evidence    string `json:"evidence"`
	Severity    string `json:"severity"`
}

type Tailored struct {
	TargetRole    string     `json:"targetRole"`
	Headline      string     `json:"headline"`
	Summary       string     `json:"summary"`
	SelectedSkills []string  `json:"selectedSkills"`
	Sections      []TSection `json:"sections"`
	Gaps          []Gap      `json:"gaps"`
	WhatChanged   []string   `json:"whatChanged"`
}

func AssignIDs(p *Profile) {
	for i := range p.Items {
		p.Items[i].ID = fmt.Sprintf("item-%d", i)
		for j := range p.Items[i].Bullets {
			p.Items[i].Bullets[j].ID = fmt.Sprintf("item-%d-b-%d", i, j)
		}
	}
}

func ValidateTailored(p Profile, t Tailored) error {
	items := map[string]bool{}
	bullets := map[string]bool{}
	for _, it := range p.Items {
		items[it.ID] = true
		for _, b := range it.Bullets {
			bullets[b.ID] = true
		}
	}
	for _, s := range t.Sections {
		for _, it := range s.Items {
			if !items[it.SourceID] {
				return fmt.Errorf("tailored output references unknown profile item: %s", it.SourceID)
			}
			for _, b := range it.Bullets {
				if !bullets[b.SourceBulletID] {
					return fmt.Errorf("tailored output references unknown profile bullet: %s", b.SourceBulletID)
				}
			}
		}
	}
	return nil
}

var nonAlnum = regexp.MustCompile(`[^a-zA-Z0-9]+`)

func Filename(name, role string) string {
	clean := func(s string) string { return strings.Trim(nonAlnum.ReplaceAllString(s, "_"), "_") }
	return clean(name) + "_" + clean(role) + ".pdf"
}
