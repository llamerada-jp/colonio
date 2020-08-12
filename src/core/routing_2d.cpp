/*
 * Copyright 2017-2020 Yuji Ito <llamerada.jp@gmail.com>
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

#include "routing_2d.hpp"

#include <cassert>
#include <delaunay_triangulation.hpp>
#include <list>

#include "context.hpp"
#include "convert.hpp"
#include "coord_system.hpp"
#include "logger.hpp"
#include "packet.hpp"
#include "utils.hpp"

namespace colonio {

Routing2D::Routing2D(Context& context_, RoutingAlgorithm2DDelegate& delegate_, const CoordSystem& coord_system_) :
    RoutingAlgorithm("2D"), context(context_), delegate(delegate_), coord_system(coord_system_) {
}

const std::set<NodeID>& Routing2D::get_required_nodes() {
  return nearby_nids;
}

void Routing2D::on_change_local_position(const Coordinate& position) {
  update_node_infos();
}

void Routing2D::on_recv_packet(const NodeID& nid, const Packet& packet) {
  // Ignore
}

void Routing2D::send_routing_info(RoutingProtocol::RoutingInfo* param) {
  if (coord_system.get_local_position().is_enable()) {
    coord_system.get_local_position().to_pb(param->mutable_r2d_position());

    for (auto& nid : connected_nodes) {
      auto known_node = known_nodes.find(nid);
      if (known_node != known_nodes.end()) {
        known_node->second.to_pb((*param->mutable_nodes())[nid.to_str()].mutable_r2d_position());
      }
    }
  }
}

bool Routing2D::update_routing_info(
    const std::set<NodeID>& online_links, bool has_update_ol,
    const std::map<NodeID, std::tuple<std::unique_ptr<const Packet>, RoutingProtocol::RoutingInfo>>& routing_infos) {
  // update connected_nodes
  connected_nodes = online_links;

  // update known_nodes
  known_nodes.clear();
  known_nodes.insert(std::make_pair(context.local_nid, coord_system.get_local_position()));

  for (auto& it : routing_infos) {
    const RoutingProtocol::RoutingInfo& routing_info = std::get<1>(it.second);
    if (routing_info.has_r2d_position()) {
      Coordinate coord = Coordinate::from_pb(routing_info.r2d_position());
      known_nodes.insert(std::make_pair(it.first, coord));
    }
  }

  for (auto& it1 : routing_infos) {
    const RoutingProtocol::RoutingInfo& routing_info = std::get<1>(it1.second);
    for (auto& it2 : routing_info.nodes()) {
      if (it2.second.has_r2d_position()) {
        known_nodes.insert(std::make_pair(NodeID::from_str(it2.first), Coordinate::from_pb(it2.second.r2d_position())));
      }
    }
  }

  // update nearby_nodes, required_nodes
  update_node_infos();

  // Ignore
  //*
#ifndef NDEBUG
  picojson::object nodes;
  picojson::array links;
  std::list<std::pair<const NodeID&, const NodeID&>> link_tmp;

  for (auto& nid : connected_nodes) {
    if (known_nodes.find(nid) != known_nodes.end()) {
      nodes.insert(std::make_pair(nid.to_str(), Convert::coordinate2json(known_nodes.at(nid))));
      const NodeID& n1 = NodeID::THIS;
      const NodeID& n2 = (nid == context.local_nid ? NodeID::THIS : nid);
      link_tmp.push_back(
          n1 < n2 ? std::make_pair(std::ref(n1), std::ref(n2)) : std::make_pair(std::ref(n2), std::ref(n1)));
    }
  }

  link_tmp.sort();
  link_tmp.unique();

  for (auto& it : link_tmp) {
    picojson::array one_pair;
    one_pair.push_back(it.first.to_json());
    one_pair.push_back(it.second.to_json());
    links.push_back(picojson::value(one_pair));
  }

  logd("routing 2d").map("nodes", picojson::value(nodes)).map("links", picojson::value(links));
#endif

  // return is_changed;
  return true;  // TODO
}

const NodeID& Routing2D::get_relay_nid(const Coordinate& position) {
  const NodeID* near_nid    = &NodeID::THIS;
  double min_distance       = std::numeric_limits<double>::max();
  Coordinate local_position = coord_system.get_local_position();

  for (auto& nid : connected_nodes) {
    auto it = known_nodes.find(nid);
    if (it != known_nodes.end()) {
      Coordinate& coord = it->second;
      double distance   = coord_system.get_distance(local_position, coord);
      if (distance < min_distance) {
        min_distance = distance;
        near_nid     = &nid;
      }
    }
  }

  return *near_nid;
}

void Routing2D::check_duplicate_point(
    const std::vector<NodeID>& nids, std::vector<double>* x_vec, std::vector<double>* y_vec) {
  std::map<Coordinate, int> checked;
  std::set<int> duplicated;

  for (int idx = 0; idx < nids.size(); idx++) {
    Coordinate coord(x_vec->at(idx), y_vec->at(idx));
    if (checked.find(coord) == checked.end()) {
      checked.insert(std::make_pair(coord, idx));
    } else {
      duplicated.insert(checked.at(coord));
      duplicated.insert(idx);
    }
  }

  for (int idx : duplicated) {
    shift_duplicate_point(nids.at(idx), &x_vec->at(idx), &y_vec->at(idx));
  }
}

void Routing2D::shift_duplicate_point(const NodeID& nid, double* x, double* y) {
  uint64_t id0;
  uint64_t id1;
  nid.get_raw(&id0, &id1);
  *x += (coord_system.PRECISION * id0) / static_cast<double>(UINT64_MAX);
  *y += (coord_system.PRECISION * id1) / static_cast<double>(UINT64_MAX);
}

void Routing2D::update_node_infos() {
  std::vector<NodeID> nids(known_nodes.size());
  std::vector<double> x_orig_vec(known_nodes.size());
  std::vector<double> y_orig_vec(known_nodes.size());
  std::vector<double> x_shift_vec(known_nodes.size());
  std::vector<double> y_shift_vec(known_nodes.size());

  // convert known nodes to coordinate vector & shift it
  int idx         = 0;
  int local_idx   = 0;
  Coordinate base = coord_system.get_local_position();
  for (auto& it : known_nodes) {
    if (it.first == context.local_nid) {
      local_idx = idx;
    }
    nids[idx]        = it.first;
    x_orig_vec[idx]  = it.second.x;
    y_orig_vec[idx]  = it.second.y;
    Coordinate shift = coord_system.shift_for_routing_2d(base, Coordinate(it.second.x, it.second.y));
    x_shift_vec[idx] = shift.x;
    y_shift_vec[idx] = shift.y;
    idx++;
  }

  // shift duplicate coordinate
  check_duplicate_point(nids, &x_shift_vec, &y_shift_vec);

  // calculate delaunay triangle
  delaunay::DelaunayTriangulation dt(x_shift_vec, y_shift_vec);
  dt.execute(0.0, 0.0, 1);
  std::vector<delaunay::Edge> edges = dt.get_edges();

  std::map<NodeID, Coordinate> old_nearby_nodes = nearby_nodes;
  nearby_nodes.clear();
  nearby_nids.clear();

  for (auto& edge : edges) {
    if (edge.first == local_idx) {
      nearby_nids.insert(nids.at(edge.second));
      nearby_nodes.insert(
          std::make_pair(nids.at(edge.second), Coordinate(x_orig_vec.at(edge.second), y_orig_vec.at(edge.second))));
    } else if (edge.second == local_idx) {
      nearby_nids.insert(nids.at(edge.first));
      nearby_nodes.insert(
          std::make_pair(nids.at(edge.first), Coordinate(x_orig_vec.at(edge.first), y_orig_vec.at(edge.first))));
    }
  }

  bool is_changed_nearby          = false;
  bool is_changed_nearby_position = false;

  if (old_nearby_nodes.size() == nearby_nodes.size()) {
    for (auto& old_node : old_nearby_nodes) {
      auto it = nearby_nodes.find(old_node.first);
      if (it == nearby_nodes.end()) {
        is_changed_nearby          = true;
        is_changed_nearby_position = true;
        break;
      }

      if (old_node.second != it->second) {
        is_changed_nearby_position = true;
      }
    }
  } else {
    is_changed_nearby          = true;
    is_changed_nearby_position = true;
  }

  if (is_changed_nearby) {
    delegate.algorithm_2d_on_change_nearby(*this, nearby_nids);
  }
  if (is_changed_nearby_position) {
    delegate.algorithm_2d_on_change_nearby_position(*this, nearby_nodes);
  }

#ifndef NDEBUG
  {
    picojson::object o;
    for (auto& it : nearby_nodes) {
      o.insert(std::make_pair(it.first.to_str(), Convert::coordinate2json(it.second)));
    }
    logd("routing 2d required").map("nids", picojson::value(o));
  }
#endif
}
}  // namespace colonio
