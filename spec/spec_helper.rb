# frozen_string_literal: true

require 'rspec'
require 'bosh/template/test'

class LinkConfiguration
  attr_reader :description, :property, :link_path, :link_namespace, :parsed_yaml_property

  def initialize(description:, property:, link_path:, parsed_yaml_property:, link_namespace: nil)
    @description = description
    @property = property
    @link_path = link_path
    @link_namespace = link_namespace || link_path.split('.').first
    @parsed_yaml_property = parsed_yaml_property
  end
end

shared_examples 'overridable_link' do |link_config|
  def get_at_property(hash, property)
    property_chain = property.split('.')

    get_this = hash
    property_chain.each do |getter|
      getter = getter.to_i if get_this.is_a?(Array)
      get_this = get_this.fetch(getter)
    end

    get_this
  end

  def set_at_property(hash, property, value)
    property_chain = property.split('.')

    getters = property_chain[0..-2]
    setter = property_chain.last

    set_this = hash
    getters.each do |getter|
      getter = getter.to_i if set_this.is_a?(Array)
      set_this = set_this.fetch(getter)
    end

    set_this.store(setter, value)
  end

  def delete_at_property(hash, property)
    property_chain = property.split('.')

    getters = property_chain[0..-2]
    setter = property_chain.last

    set_this = hash
    getters.each do |getter|
      getter = getter.to_i if set_this.is_a?(Array)
      set_this = set_this.fetch(getter)
    end

    set_this.delete(setter)
  end

  context 'when the link is not provided' do
    context 'when the property is set' do
      before do
        # TODO: constant it
        set_at_property(deployment_manifest_fragment, link_config.property, property_value)
      end

      it 'should prefer the value in the properties' do
        expect(get_at_property(parsed_yaml, link_config.parsed_yaml_property)).to eq(property_value)
      end
    end

    context 'when the property is not set' do
      before do
        delete_at_property(deployment_manifest_fragment, link_config.property)
      end

      it 'should error' do
        msg = "#{link_config.description} not found in properties nor in \"#{link_config.link_namespace}\" link. This value can be specified using the \"#{link_config.property}\" property."
        expect { parsed_yaml }.to raise_error(RuntimeError, msg)
      end
    end
  end

  context 'when the link is provided' do
    def make_properties(link, link_property)
      property_chain = link.split('.').reverse

      property_chain.reduce(link_property) do |memo, obj|
        { obj => memo }
      end
    end

    let(:links) do
      [
        Bosh::Template::Test::Link.new(
          name: link_config.link_namespace,
          properties: make_properties(link_config.link_path, link_value)
        )
      ]
    end

    let(:rendered_template) { template.render(deployment_manifest_fragment, consumes: links) }

    context 'when the property is set' do
      before do
        set_at_property(deployment_manifest_fragment, link_config.property, property_value)
      end

      it 'should prefer the value in the properties' do
        expect(get_at_property(parsed_yaml, link_config.parsed_yaml_property)).to eq(property_value)
      end
    end

    context 'when the property is not set' do
      before do
        delete_at_property(deployment_manifest_fragment, link_config.property)
      end

      it 'should render the value from the link' do
        expect(get_at_property(parsed_yaml, link_config.parsed_yaml_property)).to eq(link_value)
      end
    end
  end
end
