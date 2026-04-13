package game

import (
	"fmt"
	"math/rand"
)

type Suit uint8 

const (
	Spades Suit = iota 
	Hearts
	Diamonds
	Clubs
)

func (s Suit) String() string {
	return [...]string{"♠", "♥", "♦", "♣"}[s]
}

type Rank uint8 

const (
	Two Rank = iota + 2
	Three
	Four
	Five
	Six
	Seven
	Eight
	Nine
	Ten
	Jack
	Queen
	King
	Ace
)

func (r Rank) String() string {
	switch r {
	case Two, Three, Four, Five, Six, Seven, Eight, Nine, Ten:
		return fmt.Sprintf("%d", int(r))
	case Jack:
		return "J"
	case Queen:
		return "Q"
	case King:
		return "K"
	case Ace:
		return "A"
	}
	return "?"
}

type Card struct {
	Rank Rank 
	Suit Suit 
}

func (c Card) String() string {
	return c.Rank.String() + c.Suit.String()
}

func (c Card) CardID() int {
	return int(c.Suit)*13 + int(c.Rank-2)
}

func CardFromID(id int) Card {
	return Card{
		Suit: Suit(id/13),
		Rank: Rank(id%13)+2,
	}
}

type Deck struct {
	Cards []Card
}

func NewDeck() *Deck {
	d := &Deck{}
	for s := Spades; s <= Clubs; s++ {
		for r := Two; r <= Ace; r++ {
			d.Cards = append(d.Cards, Card{Rank: r, Suit: s})
		}
	}
	return d
}

func (d *Deck) Shuffle(rng *rand.Rand) {
	rng.Shuffle(len(d.Cards), func(i, j int){
		d.Cards[i], d.Cards[j] = d.Cards[j], d.Cards[i]
	})
}

func (d *Deck) Deal() (Card, error) {
	if len(d.Cards) == 0 {
		return Card{}, fmt.Errorf("deck is empty")
	}
	c := d.Cards[0]
	d.Cards = d.Cards[1:]
	return c, nil
}
