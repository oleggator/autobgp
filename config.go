package main

import (
	"gopkg.in/yaml.v3"
	"io"
)

type DNSConfig struct {
	Listen           string `yaml:"listen"`
	Network          string `yaml:"network"`
	AuthoritativeDNS string `yaml:"authoritative_dns"`
}

type BGPNeighborsConfig struct {
	ASN    uint32 `yaml:"asn"`
	Prefix string `yaml:"prefix"`
}

type BGPConfig struct {
	ListenPort uint16             `yaml:"listen"`
	RouterID   string             `yaml:"router_id"`
	ASN        uint32             `yaml:"asn"`
	Neighbors  BGPNeighborsConfig `yaml:"neighbors"`
}

type RulesConfig struct {
	NextHop  string   `yaml:"next_hop"`
	Zones    Zones    `yaml:"zones"`
	Networks []string `yaml:"networks"`
}

type Config struct {
	DNS   DNSConfig   `yaml:"dns"`
	BGP   BGPConfig   `yaml:"bgp"`
	Rules RulesConfig `yaml:"rules"`
}

func ReadConfig(r io.Reader) (Config, error) {
	var config Config

	if err := yaml.NewDecoder(r).Decode(&config); err != nil {
		return Config{}, err
	}

	config.Rules.Zones.Normalize()

	return config, nil
}
