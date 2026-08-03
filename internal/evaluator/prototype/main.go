package main

// PROTOTYPE — throwaway TUI shell over pipeline.go. Not production code.
//
// Drive the eval-jury pipeline by hand: probe -> jury checkpoint -> run.
// Run: make proto-eval

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

const (
	bold = "\x1b[1m"
	dim  = "\x1b[2m"
	red  = "\x1b[31m"
	grn  = "\x1b[32m"
	rst  = "\x1b[0m"
)

func main() {
	seed := int64(42)
	sim := NewSim(seed, len(roster())-1) // subject: llama-3.1-8b (weak, free)
	reader := bufio.NewReader(os.Stdin)
	fmt.Print(render(sim))
	for {
		fmt.Print(dim + "> " + rst)
		line, _ := reader.ReadString('\n')
		cmd := strings.TrimSpace(line)
		switch {
		case cmd == "q":
			fmt.Println("bye")
			return
		case cmd == "" || cmd == " ":
			sim.Tick()
		case cmd == "a":
			auto(sim)
		case cmd >= "1" && cmd <= "4":
			sim.SetPolicy(Policy(cmd[0] - '1'))
		case cmd == "p":
			p, subj := sim.policy, sim.subject
			sim = NewSim(seed, subj)
			sim.policy = p
		case cmd == "r":
			seed++
			sim = NewSim(seed, sim.subject)
		case cmd == "v":
			sim.ReviveAll()
		case strings.HasPrefix(cmd, "k"):
			kill(sim, strings.TrimSpace(strings.TrimPrefix(cmd, "k")))
		case strings.HasPrefix(cmd, "s"):
			if idx := findModel(sim, strings.TrimSpace(strings.TrimPrefix(cmd, "s"))); idx >= 0 && sim.phase != PhaseRun {
				sim = NewSim(seed, idx)
			}
		}
		fmt.Print(render(sim))
	}
}

// auto fast-forwards: from PROBE to the jury checkpoint, from the checkpoint
// through the run to DONE.
func auto(s *Sim) {
	if s.phase == PhaseJury {
		s.Launch()
	}
	from := s.phase
	for i := 0; i < 20000; i++ {
		if s.phase == PhaseDone || (s.phase != from && from != PhaseJury) {
			return
		}
		s.Tick()
	}
}

func findModel(s *Sim, sub string) int {
	for i, m := range s.models {
		if sub != "" && strings.Contains(m.ID, sub) {
			return i
		}
	}
	return -1
}

func kill(s *Sim, sub string) {
	if i := findModel(s, sub); i >= 0 {
		s.KillModel(i)
	}
}

func render(s *Sim) string {
	var b strings.Builder
	b.WriteString("\033[2J\033[H")
	fmt.Fprintf(&b, "%s== EVAL JURY PIPELINE — PROTOTYPE (throwaway) ==%s  tick %d (%.1fs virtual)  phase %s%s%s\n\n",
		bold, rst, s.tick, float64(s.tick)*tickMs/1000, grn, s.phase, rst)

	// Probe table.
	b.WriteString(bold + "PROBE" + rst + " (one-click pre-flight: reachable? speed? stability?)\n")
	fmt.Fprintf(&b, "%s  %-14s %-4s %-6s %-7s %-9s %s\n", dim, "model", "ok", "succ", "tps", "$/1M io", "note"+rst)
	for i, m := range s.models {
		p := s.probes[i]
		ok, succ, tps := dim+"?"+rst, "-", "-"
		if p != nil && p.Rounds > 0 {
			succ = fmt.Sprintf("%d/%d", p.Successes, p.Rounds)
			if p.Rounds == probeRounds {
				if p.Reachable {
					ok = grn + "yes" + rst
					tps = fmt.Sprintf("%.0f", p.AvgTPS)
				} else {
					ok = red + "NO" + rst
				}
			}
		}
		note := ""
		if m.Down {
			note = red + "DOWN (injected)" + rst
		}
		if i == s.subject {
			note += " " + bold + "<- SUBJECT" + rst
		}
		fmt.Fprintf(&b, "  %-14s %-4s %-6s %-7s %.2f/%-5.2f %s\n", m.ID, ok, succ, tps, m.InPrice, m.OutPrice, note)
	}

	// Jury.
	b.WriteString("\n" + bold + "JURY" + rst + fmt.Sprintf(" (policy %s%s%s — [1]balanced [2]speed [3]iq [4]cost)\n", bold, s.policy, rst))
	if len(s.jury) == 0 {
		b.WriteString(dim + "  (not selected yet)" + rst + "\n")
	} else {
		fmt.Fprintf(&b, "  %s\n", s.juryNote)
	}

	// Pipeline.
	fmt.Fprintf(&b, "\n%sPIPELINE%s  exam %d/%d done (pending %d, inflight %d)  judge %d/%d done (pending %d, inflight %d)  circuit %d/%d\n",
		bold, rst, s.examDone, s.examTotal, len(s.examPending), len(s.examInflight),
		s.judgeDone, s.judgeTotal, len(s.judgePending), len(s.judgeInflight), s.circuit, circuitLimit)
	avgTPS := "-"
	if s.examDone > 0 && s.answerMsSum > 0 {
		avgTPS = fmt.Sprintf("%.0f tok/s", float64(s.answerTok)/(float64(s.answerMsSum)/1000))
	}
	fmt.Fprintf(&b, "  speed: answer avg %s   cost: exam $%.4f + judge $%.4f = %s$%.4f%s\n",
		avgTPS, s.examCost, s.TotalJudgeCost(), bold, s.examCost+s.TotalJudgeCost(), rst)

	// Score board with per-judge columns.
	b.WriteString("\n" + bold + "SCORES" + rst + " (per-judge columns; final = median, case = mean of sample medians)\n")
	hdr := fmt.Sprintf("%s  %-5s %-4s", dim, "case", "samp")
	for slot := 0; slot < 3; slot++ {
		name := "—"
		if slot < len(s.jury) {
			name = short(s.models[s.jury[slot]].ID)
		}
		hdr += fmt.Sprintf(" %-9s", name)
	}
	b.WriteString(hdr + fmt.Sprintf(" %-7s %-6s%s\n", "median", "spread", rst))
	for c := 0; c < numCases; c++ {
		for m := 0; m < numSamples; m++ {
			smp := s.grid[c][m]
			row := fmt.Sprintf("  %-5d %-4d", c+1, m+1)
			for slot := 0; slot < 3; slot++ {
				text, color := "-", dim
				if slot < len(s.jury) {
					switch {
					case smp.scores[slot] != nil:
						text, color = fmt.Sprintf("%.2f", *smp.scores[slot]), ""
					case smp.done[slot]:
						text, color = "FAIL", red
					}
				}
				row += " " + color + fmt.Sprintf("%-9s", text) + rst
			}
			med := dim + "-" + rst
			if smp.median != nil {
				med = fmt.Sprintf("%s%.2f%s", bold, *smp.median, rst)
			} else if smp.settled {
				med = red + "null" + rst
			}
			row += fmt.Sprintf(" %-7s", med)
			if smp.settled && smp.median != nil {
				row += fmt.Sprintf(" %.2f", smp.spread())
			}
			if m == numSamples-1 && s.caseScore[c] != nil {
				row += fmt.Sprintf("   %scase=%.2f%s", grn, *s.caseScore[c], rst)
			}
			b.WriteString(row + "\n")
		}
	}
	if s.phase == PhaseDone {
		fmt.Fprintf(&b, "\n%sFINAL: %s%s\n", bold, s.FinalString(), rst)
	}

	// Event log.
	b.WriteString("\n" + bold + "LOG" + rst + "\n")
	for _, e := range s.events {
		b.WriteString(dim + "  " + e + rst + "\n")
	}

	b.WriteString("\n" + bold + "[enter]" + rst + " tick  " + bold + "[a]" + rst + " auto  " +
		bold + "[1-4]" + rst + " policy  " + bold + "[k name]" + rst + " kill model  " +
		bold + "[v]" + rst + " revive  " + bold + "[s name]" + rst + " subject  " +
		bold + "[p]" + rst + " re-probe  " + bold + "[r]" + rst + " reset  " + bold + "[q]" + rst + " quit\n")
	return b.String()
}

func short(id string) string {
	if len(id) > 9 {
		return id[:9]
	}
	return id
}
